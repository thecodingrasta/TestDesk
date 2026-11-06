package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

var suites = map[string][]string{
	"backend-xunit":      {"Acme.Tests.Orders.CreateOrderTests.ValidOrder_passes", "Acme.Tests.Orders.CreateOrderTests.InvalidCard_fails", "Acme.Tests.Users.LoginTests.ValidCredentials_passes"},
	"frontend-vitest":    {"checkout/cart.spec.ts > CartScreen > adds item", "checkout/cart.spec.ts > CartScreen > removes item", "profile/settings.spec.ts > SettingsScreen > saves changes"},
	"browser-playwright": {"[chromium] › tests/login.spec.ts:12 › Login screen › accepts valid user", "[chromium] › tests/orders.spec.ts:23 › Orders screen › opens order details"},
}

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: fake_provider discover|run provider [artifact] [selector]")
		os.Exit(2)
	}
	mode := os.Args[1]
	provider := os.Args[2]
	tests := suites[provider]
	switch mode {
	case "discover":
		for _, test := range tests {
			fmt.Println(test)
		}
	case "run":
		if len(os.Args) < 4 {
			os.Exit(2)
		}
		artifact := os.Args[3]
		selector := ""
		if len(os.Args) > 4 {
			selector = os.Args[4]
		}
		selected := tests
		if selector != "" {
			selected = nil
			for _, test := range tests {
				if strings.EqualFold(test, selector) || strings.Contains(test, selector) || strings.Contains(selector, test) {
					selected = append(selected, test)
				}
			}
		}
		if len(selected) == 0 {
			fmt.Fprintln(os.Stderr, "no fake tests matched")
			os.Exit(1)
		}
		failed := false
		var b strings.Builder
		b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><testsuite name="` + provider + `">`)
		for _, test := range selected {
			fmt.Println("running", test)
			time.Sleep(30 * time.Millisecond)
			className := provider
			name := test
			b.WriteString(`<testcase classname="` + xmlEscape(className) + `" name="` + xmlEscape(name) + `" time="0.03">`)
			if strings.Contains(strings.ToLower(test), "fails") {
				failed = true
				b.WriteString(`<failure message="intentional fake failure">stack trace for ` + xmlEscape(test) + `</failure>`)
			}
			b.WriteString(`</testcase>`)
		}
		b.WriteString(`</testsuite>`)
		if err := os.WriteFile(artifact, []byte(b.String()), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if failed {
			os.Exit(1)
		}
	default:
		os.Exit(2)
	}
}

func xmlEscape(value string) string {
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, "<", "&lt;")
	value = strings.ReplaceAll(value, ">", "&gt;")
	value = strings.ReplaceAll(value, `"`, "&quot;")
	return value
}
