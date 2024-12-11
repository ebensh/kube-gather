package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"strings"

	_ "github.com/mattn/go-sqlite3"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/api/core/v1"
)

func main() {
	// Parse command-line arguments
	resourcesArg := flag.String("resources", "", "List (one per line) of namespace:resourceType:resourceName")
	dbFile := flag.String("db", "kube_data.db", "Path to the SQLite database file")
	flag.Parse()

	if *resourcesArg == "" {
		log.Fatalf("No resources provided. Use the --resources flag to specify resources.")
	}
	resources := strings.Split(*resourcesArg, "\n")

	clientConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		log.Fatalf("Error loading kube client config: %v", err)
	}

	// Create Kubernetes client
	clientset, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		log.Fatalf("Error creating Kubernetes client: %v", err)
	}

	// Initialize SQLite database
	db, err := sql.Open("sqlite3", *dbFile)
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}
	defer db.Close()

	err = initializeDatabase(db)
	if err != nil {
		log.Fatalf("Error initializing database: %v", err)
	}

	// Process each resource
	for _, res := range resources {
		parts := strings.Split(res, ":")
		if len(parts) != 3 {
			log.Printf("Invalid resource format: %s\n", res)
			continue
		}

		namespace, resourceType, resourceName := parts[0], parts[1], parts[2]

		switch resourceType {
		case "deployment":
			processDeployment(clientset, db, namespace, resourceName)
		case "configmap":
			processConfigMap(clientset, db, namespace, resourceName)
		case "secret":
			processSecret(clientset, db, namespace, resourceName)
		default:
			log.Printf("Unsupported resource type: %s\n", resourceType)
		}
	}
}

func initializeDatabase(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS deployments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			namespace TEXT,
			name TEXT,
			spec TEXT,
			status TEXT
		);
	`)
	if err != nil {
		return fmt.Errorf("Error creating deployments table: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS deployment_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			deployment_id INTEGER,
			logs BLOB,
			FOREIGN KEY(deployment_id) REFERENCES deployments(id)
		);
	`)
	if err != nil {
		return fmt.Errorf("Error creating deployment_logs table: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS configmaps (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			namespace TEXT,
			name TEXT,
			data TEXT
		);
	`)
	if err != nil {
		return fmt.Errorf("Error creating configmaps table: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS secrets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			namespace TEXT,
			name TEXT,
			data TEXT
		);
	`)
	if err != nil {
		return fmt.Errorf("Error creating secrets table: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS deployment_dependencies (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			deployment_id INTEGER,
			resource_type TEXT,
			resource_id INTEGER,
			FOREIGN KEY(deployment_id) REFERENCES deployments(id)
		);
	`)
	return err
}

func processDeployment(clientset *kubernetes.Clientset, db *sql.DB, namespace, name string) {
	fmt.Printf("Processing deployment: %s/%s\n", namespace, name)

	deployment, err := clientset.AppsV1().Deployments(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		log.Printf("Error fetching deployment: %v\n", err)
		return
	}

	specBytes, err := json.Marshal(deployment.Spec)
	if err != nil {
		log.Printf("Error marshalling deployment spec: %v\n", err)
		return
	}

	statusBytes, err := json.Marshal(deployment.Status)
	if err != nil {
		log.Printf("Error marshalling deployment status: %v\n", err)
		return
	}

	result, err := db.Exec(`
		INSERT INTO deployments (namespace, name, spec, status) VALUES (?, ?, ?, ?)
	`, namespace, name, string(specBytes), string(statusBytes))
	if err != nil {
		log.Printf("Error inserting deployment into database: %v\n", err)
		return
	}

	deploymentID, err := result.LastInsertId()
	if err != nil {
		log.Printf("Error getting last insert ID: %v\n", err)
		return
	}

	processDeploymentLogs(clientset, db, namespace, name, deploymentID)
	//linkDependentResources(db, namespace, deployment, deploymentID)
}

func processDeploymentLogs(clientset *kubernetes.Clientset, db *sql.DB, namespace, deploymentName string, deploymentID int64) {
	pods, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app=%s", deploymentName),
	})
	if err != nil {
		log.Printf("Error listing pods: %v\n", err)
		return
	}

	var logsBuffer bytes.Buffer

	for _, pod := range pods.Items {
		logStream, err := clientset.CoreV1().Pods(namespace).GetLogs(pod.Name, &corev1.PodLogOptions{}).Stream(context.TODO())
		if err != nil {
			log.Printf("Error fetching logs for pod %s: %v\n", pod.Name, err)
			continue
		}
		defer logStream.Close()

		buf := new(bytes.Buffer)
		buf.ReadFrom(logStream)
		logsBuffer.Write(buf.Bytes())
	}

	_, err = db.Exec(`
		INSERT INTO deployment_logs (deployment_id, logs) VALUES (?, ?)
	`, deploymentID, logsBuffer.Bytes())
	if err != nil {
		log.Printf("Error inserting logs into database: %v\n", err)
	}
}

func processConfigMap(clientset *kubernetes.Clientset, db *sql.DB, namespace, name string) {
	fmt.Printf("Processing configmap: %s/%s\n", namespace, name)

	configMap, err := clientset.CoreV1().ConfigMaps(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		log.Printf("Error fetching configmap: %v\n", err)
		return
	}

	dataBytes, err := json.Marshal(configMap.Data)
	if err != nil {
		log.Printf("Error marshalling configmap data: %v\n", err)
		return
	}

	result, err := db.Exec(`
		INSERT INTO configmaps (namespace, name, data) VALUES (?, ?, ?)
	`, namespace, name, string(dataBytes))
	if err != nil {
		log.Printf("Error inserting configmap into database: %v\n", err)
		return
	}

	configMapID, err := result.LastInsertId()
	if err != nil {
		log.Printf("Error getting last insert ID: %v\n", err)
		return
	}

	// TODO: Link to dependent deployments if applicable
	fmt.Printf("ConfigMap %s/%s processed and stored with ID %d\n", namespace, name, configMapID)
}

func processSecret(clientset *kubernetes.Clientset, db *sql.DB, namespace, name string) {
	fmt.Printf("Processing secret: %s/%s\n", namespace, name)

	secret, err := clientset.CoreV1().Secrets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		log.Printf("Error fetching secret: %v\n", err)
		return
	}

	dataBytes, err := json.Marshal(secret.Data)
	if err != nil {
		log.Printf("Error marshalling secret data: %v\n", err)
		return
	}

	result, err := db.Exec(`
		INSERT INTO secrets (namespace, name, data) VALUES (?, ?, ?)
	`, namespace, name, string(dataBytes))
	if err != nil {
		log.Printf("Error inserting secret into database: %v\n", err)
		return
	}

	secretID, err := result.LastInsertId()
	if err != nil {
		log.Printf("Error getting last insert ID: %v\n", err)
		return
	}

	// TODO: Link to dependent deployments if applicable
	fmt.Printf("Secret %s/%s processed and stored with ID %d\n", namespace, name, secretID)
}

// func linkDependentResources(db *sql.DB, deploymentID int64, resourceType string, resourceID int64) {
// 	_, err := db.Exec(`
// 		INSERT INTO deployment_dependencies (deployment_id, resource_type, resource_id)
// 		VALUES (?, ?, ?)
// 	`, deploymentID, resourceType, resourceID)
// 	if err != nil {
// 		log.Printf("Error linking dependent resource (type: %s, id: %d) to deployment (id: %d): %v\n",
// 			resourceType, resourceID, deploymentID, err)
// 	} else {
// 		fmt.Printf("Linked %s (ID %d) to Deployment ID %d\n", resourceType, resourceID, deploymentID)
// 	}
// }