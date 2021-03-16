package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/hashicorp/vault/api"

	"database/sql"

	_ "github.com/lib/pq"
)

const functionName = "demo-function"

// Payload captures the basic payload we're sending for demonstration
// Ex: {"payload": "hello"}
type Payload struct {
	Message string `json:"payload"`
}

// String prints the payload recieved
func (m Payload) String() string {
	return m.Message
}

func handle(ctx context.Context, payload Payload) error {
	logger := log.New(os.Stderr, fmt.Sprintf("[%s] ", functionName), 0)
	err := handleRequest(ctx, payload, logger)
	if err != nil {
		logger.Println("Error handling request", err)
	}
	return err
}

// handleRequest reads credentials from /tmp and uses them to query the database
// for users. The database is determined by the DATABASE_URL environment
// variable, and the username and password are retrieved from the secret.
func handleRequest(ctx context.Context, payload Payload, logger *log.Logger) error {
	logger.Println("Received:", payload.String())
	logger.Println("Reading file /tmp/vault_secret.json")
	secretRaw, err := ioutil.ReadFile("/tmp/vault_secret.json")
	if err != nil {
		return fmt.Errorf("error reading file: %w", err)
	}

	var secret api.Secret
	err = json.Unmarshal(secretRaw, &secret)
	if err != nil {
		return err
	}

	logger.Println("Querying users using credentials from disk")
	err = readUsersFromDatabase(ctx, logger, &secret)
	if err != nil {
		logger.Println("Failed to read users from database", err)
	}

	proxyClient, err := api.NewClient(&api.Config{
		Address: "http://127.0.0.1:8200",
	})
	if err != nil {
		return err
	}
	proxySecret, err := proxyClient.Logical().Read(os.Getenv("VAULT_SECRET_PATH_DB"))
	if err != nil {
		return err
	}

	logger.Println("Querying users using credentials from proxy")
	err = readUsersFromDatabase(ctx, logger, proxySecret)
	if err != nil {
		logger.Println("Failed to read users from database", err)
	}

	return nil
}

func readUsersFromDatabase(ctx context.Context, logger *log.Logger, secret *api.Secret) error {
	connStr := fmt.Sprintf("postgres://%s:%s@%s/lambdadb?sslmode=disable", secret.Data["username"], secret.Data["password"], os.Getenv("DATABASE_URL"))
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return err
	}
	defer db.Close()

	var users []string
	rows, err := db.QueryContext(ctx, "SELECT usename FROM pg_catalog.pg_user")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var user string
		if err = rows.Scan(&user); err != nil {
			return err
		}
		users = append(users, user)
	}
	logger.Println("users: ")
	for i := range users {
		logger.Println("    ", users[i])
	}

	return nil
}

func main() {
	lambda.Start(handle)
}
