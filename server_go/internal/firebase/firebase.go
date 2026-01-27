package firebase

import (
	"context"
	"fmt"
	"log"
	"os"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	"google.golang.org/api/option"
)

type Client struct {
	App  *firebase.App
	Auth *auth.Client
}

func InitFirebase() (*Client, error) {
	ctx := context.Background()

	serviceAccountPath := os.Getenv("FIREBASE_SERVICE_ACCOUNT")
	if serviceAccountPath == "" {
		// Fallback to default path if not specified
		serviceAccountPath = "firebase-service-account.json"
	}

	var app *firebase.App
	var err error

	// Try to load credentials from file if it exists
	if data, readErr := os.ReadFile(serviceAccountPath); readErr == nil {
		log.Printf("[FIREBASE] Initializing with service account from: %s", serviceAccountPath)
		// Use type-specific option to mitigate risk of malformed/unexpected JSON types
		opt := option.WithAuthCredentialsJSON(option.ServiceAccount, data)
		app, err = firebase.NewApp(ctx, nil, opt)
	} else {
		log.Printf("[FIREBASE] INFO: Service account file not found or unreadable at %s. Falling back to default credentials.", serviceAccountPath)
		// Rely on GOOGLE_APPLICATION_CREDENTIALS or other default mechanisms
		app, err = firebase.NewApp(ctx, nil)
	}

	if err != nil {
		return nil, fmt.Errorf("error initializing firebase app: %v", err)
	}

	authClient, err := app.Auth(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting firebase auth client: %v", err)
	}

	return &Client{
		App:  app,
		Auth: authClient,
	}, nil
}

func (c *Client) VerifyIDToken(ctx context.Context, idToken string) (*auth.Token, error) {
	return c.Auth.VerifyIDToken(ctx, idToken)
}
