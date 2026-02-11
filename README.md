# grantsy-go

Go SDK for the [Grantsy](https://grantsy.dev) Entitlements API.

## Installation

```bash
go get github.com/grantsy/grantsy-go
```

## Usage

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"log"

	grantsy "github.com/grantsy/grantsy-go"
)

func main() {
	client, err := grantsy.New("https://grantsy.example.com", "your-api-key")
	if err != nil {
		log.Fatal(err)
	}

	// Check if a user has access to a feature
	result, err := client.CheckAccess(context.Background(), grantsy.CheckAccessParams{
		UserID:  "user-123",
		Feature: "premium-export",
	})
	if err != nil {
		var apiErr *grantsy.APIError
		if errors.As(err, &apiErr) {
			log.Printf("API error %d: %s", apiErr.Status, apiErr.Detail)
		}
		log.Fatal(err)
	}

	fmt.Println("Allowed:", result.Allowed)
}
```

### List Features

```go
// List features available to a user
features, err := client.ListFeatures(context.Background(), grantsy.ListFeaturesParams{
	UserID: "user-123",
})
if err != nil {
	log.Fatal(err)
}

fmt.Println("Plan:", features.Plan)
for _, f := range features.Features {
	fmt.Println("Feature:", f)
}
```

### List Plans

```go
// List available plans with feature details
plans, err := client.ListPlans(context.Background(), grantsy.ListPlansParams{
	Expand: "features",
})
if err != nil {
	log.Fatal(err)
}

for _, p := range plans.Plans {
	fmt.Printf("Plan: %s (%s)\n", p.Name, p.ID)
	for _, v := range p.Variants {
		fmt.Printf("  Variant: %s - %s/%s\n", v.Name, v.Price, v.Interval)
	}
}
```

### Get Subscription

```go
// Get a user's subscription
sub, err := client.GetSubscription(context.Background(), grantsy.GetSubscriptionParams{
	UserID: "user-123",
})
if err != nil {
	log.Fatal(err)
}

fmt.Println("Has subscription:", sub.HasSubscription)
fmt.Println("Plan:", sub.Plan)
fmt.Println("Status:", sub.Status)
for _, f := range sub.Features {
	fmt.Println("Feature:", f)
}
```

### Raw Subscription Data

Access provider-specific subscription details via the `Raw` field:

```go
if sub.HasSubscription && sub.Raw.Provider == grantsy.ProviderLemonSqueezy {
	ls, err := sub.Raw.Data.AsLemonSqueezySubscriptionDTO()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("LemonSqueezy ID:", ls.ID)
	fmt.Println("Product:", ls.ProductName)
	fmt.Println("Status:", ls.StatusFormatted)
	fmt.Println("Card:", ls.CardBrand, ls.CardLastFour)
}
```

## Regenerating the SDK

Requires [Task](https://taskfile.dev):

```bash
task generate
```

This downloads the latest OpenAPI spec and regenerates the Go client code.
