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

	// prioritize service account file if path is provided and file exists
	if serviceAccountPath != "" {
		if data, readErr := os.ReadFile(serviceAccountPath); readErr == nil {
			log.Printf("[FIREBASE] Initializing with service account from: %s", serviceAccountPath)
			opt := option.WithAuthCredentialsJSON(option.ServiceAccount, data)
			app, err = firebase.NewApp(ctx, nil, opt)
		}
	}

	// Fallback to Application Default Credentials (ADC)
	if app == nil {
		log.Printf("[FIREBASE] Initializing with Application Default Credentials (ADC)")
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

func (c *Client) DeleteUser(ctx context.Context, uid string) error {
	return c.Auth.DeleteUser(ctx, uid)
}
