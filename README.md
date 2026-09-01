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
	result, _, err := client.Check(context.Background(), grantsy.CheckParams{
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

	fmt.Println("Allowed:", result.Data.Allowed)
}
```

All methods return `(*Result[T], *http.Response, error)`. The `Result` envelope contains `Data` (the parsed payload) and `Meta` (request ID, timestamp, API version). The `*http.Response` provides access to HTTP headers and status code.

### Localized name & description

Plan and feature `Name`/`Description` are localized server-side. Set the `AcceptLanguage` field on any call's params to choose the language; it is sent as the `Accept-Language` header. The value may be a single BCP-47 tag (`"es"`, `"en-US"`) or a full list (`"es,en;q=0.8"`). When omitted, the server returns its configured `default_language`.

```go
result, _, err := client.Plans.List(context.Background(), grantsy.PlanListParams{
	AcceptLanguage: "es",
})
if err != nil {
	log.Fatal(err)
}

for _, p := range result.Data {
	fmt.Printf("Plan: %s (%s)\n", p.Name, p.ID) // p.Name in Spanish
}
```

### List Features

```go
result, _, err := client.Features.List(context.Background(), grantsy.FeatureListParams{})
if err != nil {
	log.Fatal(err)
}

for _, f := range result.Data {
	fmt.Printf("Feature: %s (%s)\n", f.Name, f.ID)
}
```

### Get Feature

```go
result, _, err := client.Features.Get(context.Background(), grantsy.FeatureGetParams{
	FeatureID: "dashboard",
})
if err != nil {
	log.Fatal(err)
}

fmt.Printf("Feature: %s (%s)\n", result.Data.Name, result.Data.ID)
```

### List Plans

```go
result, _, err := client.Plans.List(context.Background(), grantsy.PlanListParams{
	Expand: []grantsy.PlansExpand{grantsy.PlansExpandFeatures},
})
if err != nil {
	log.Fatal(err)
}

for _, p := range result.Data {
	fmt.Printf("Plan: %s (%s)\n", p.Name, p.ID)
	for _, f := range p.Features {
		fmt.Printf("  Feature: %s\n", f.Name)
	}
}
```

### Get User

```go
result, _, err := client.Users.Get(context.Background(), grantsy.UserGetParams{
	UserID: "user-123",
	Expand: []grantsy.UserExpand{
		grantsy.UserExpandPlan,
		grantsy.UserExpandFeatures,
		grantsy.UserExpandSubscription,
	},
})
if err != nil {
	log.Fatal(err)
}

fmt.Println("Plan:", result.Data.PlanID)
for _, f := range result.Data.Features {
	fmt.Println("Feature:", f.Name)
}
```

### Raw Subscription Data

Access provider-specific subscription details via the `Raw` field on a user's subscription:

```go
sub := result.Data.Subscription
if sub != nil && sub.Raw.Provider == grantsy.ProviderLemonSqueezy {
	ls := sub.Raw.Data.LemonSqueezySubscription
	fmt.Println("LemonSqueezy ID:", ls.ID)
	fmt.Println("Product:", ls.ProductName)
	fmt.Println("Status:", ls.StatusFormatted)
	fmt.Println("Card:", ls.CardBrand, ls.CardLastFour)
}
```

### Refunds

A refunded subscription revokes access regardless of its status. `RefundedAt` is a
`Nullable[int]` holding the Unix timestamp of the refund; `Get` returns an error when
the field is null or absent:

```go
sub := result.Data.Subscription
if sub != nil {
	if ts, err := sub.RefundedAt.Get(); err == nil {
		fmt.Println("Refunded at:", time.Unix(int64(ts), 0))
	}
}
```

## Regenerating the SDK

Requires [Task](https://taskfile.dev):

```bash
task generate
```

This downloads the latest OpenAPI spec and regenerates the Go client code.
