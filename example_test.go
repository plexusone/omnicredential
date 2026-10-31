package omnicredential_test

import (
	"fmt"
	"time"

	"github.com/plexusone/omnicredential"
)

// Build a registry in Go: one function describes a credential, and a loop
// makes an entry for each environment.
func Example() {
	sessionKey := func(env string) omnicredential.Credential {
		return omnicredential.Credential{
			ID:          "example-app/" + env + "/SESSION_KEY",
			Name:        "SESSION_KEY",
			Description: "Encrypts example-app's stored sessions.",
			System:      "example-app",
			Environment: env,
			Kind:        omnicredential.KindRandomSecret,
			Status:      omnicredential.StatusActive,
			Sensitivity: omnicredential.SensitivityHigh,
			Owner:       "platform team",
			Issuer:      omnicredential.Issuer{Type: omnicredential.IssuerSelf},
			Generation: omnicredential.Generation{
				Method:  omnicredential.MethodRandom,
				Spec:    "32 random bytes, hex",
				Where:   "the operator's machine, straight into the parameter store",
				Command: "openssl rand -hex 32",
			},
			Entitlements: []omnicredential.Entitlement{{Resource: "example-app sessions", Access: "decrypt and forge"}},
			Stores: []omnicredential.Store{{
				Type: omnicredential.StoreAWSSSM, Role: omnicredential.RolePrimary,
				Path: "/example/" + env + "/SESSION_KEY",
			}},
			Consumers: []omnicredential.Consumer{{System: "example-app", Component: "api", Via: "env SESSION_KEY"}},
			Rotation: omnicredential.Rotation{
				Policy: omnicredential.PolicyScheduled, IntervalDays: 180,
				Impact: "everyone is signed out once",
			},
			Handling: omnicredential.Handling{
				Generate: omnicredential.ActorHuman, Distribute: omnicredential.ActorSystem, Rotate: omnicredential.ActorHuman,
				Automation: "the deploy tool renders it onto the server",
			},
			Revocation: "Rotate it and restart the service.",
		}
	}
	reg := omnicredential.NewRegistry()
	for _, env := range []string{"staging", "prod"} {
		reg.Add(sessionKey(env))
	}
	if err := reg.Validate(); err != nil {
		panic(err)
	}

	log := omnicredential.NewLog()
	created := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	if err := log.Append(reg, omnicredential.Event{ //nolint:gosec // G101: the field holds a credential ID, not a value.
		Credential: "example-app/prod/SESSION_KEY", Type: omnicredential.EventCreated, At: created, By: "ops",
	}); err != nil {
		panic(err)
	}

	now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	for _, c := range reg.Credentials {
		a := omnicredential.Assess(c, log.Record(c.ID), now, omnicredential.DefaultWarn)
		fmt.Printf("%s: %s (%s)\n", c.ID, a.State, a.Reason)
	}
	// Output:
	// example-app/staging/SESSION_KEY: unrecorded (active, but no created or rotated event)
	// example-app/prod/SESSION_KEY: due (rotation due 2026-07-14)
}
