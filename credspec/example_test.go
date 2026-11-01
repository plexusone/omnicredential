package credspec_test

import (
	"fmt"
	"time"

	"github.com/plexusone/systemspec-credential/credspec"
)

// Build a registry in Go: one function describes a credential, and a loop
// makes an entry for each environment.
func Example() {
	sessionKey := func(env string) credspec.Credential {
		return credspec.Credential{
			ID:          "example-app/" + env + "/SESSION_KEY",
			Name:        "SESSION_KEY",
			Description: "Encrypts example-app's stored sessions.",
			System:      "example-app",
			Environment: env,
			Kind:        credspec.KindRandomSecret,
			Status:      credspec.StatusActive,
			Sensitivity: credspec.SensitivityHigh,
			Owner:       "platform team",
			Issuer:      credspec.Issuer{Type: credspec.IssuerSelf},
			Generation: credspec.Generation{
				Method:  credspec.MethodRandom,
				Spec:    "32 random bytes, hex",
				Where:   "the operator's machine, straight into the parameter store",
				Command: "openssl rand -hex 32",
			},
			Entitlements: []credspec.Entitlement{{Resource: "example-app sessions", Access: "decrypt and forge"}},
			Stores: []credspec.Store{{
				Type: credspec.StoreAWSSSM, Role: credspec.RolePrimary,
				Path: "/example/" + env + "/SESSION_KEY",
			}},
			Consumers: []credspec.Consumer{{System: "example-app", Component: "api", Via: "env SESSION_KEY"}},
			Rotation: credspec.Rotation{
				Policy: credspec.PolicyScheduled, IntervalDays: 180,
				Impact: "everyone is signed out once",
			},
			Handling: credspec.Handling{
				Generate: credspec.ActorHuman, Distribute: credspec.ActorSystem, Rotate: credspec.ActorHuman,
				Automation: "the deploy tool renders it onto the server",
			},
			Revocation: "Rotate it and restart the service.",
		}
	}
	reg := credspec.NewRegistry()
	for _, env := range []string{"staging", "prod"} {
		reg.Add(sessionKey(env))
	}
	if err := reg.Validate(); err != nil {
		panic(err)
	}

	log := credspec.NewLog()
	created := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	if err := log.Append(reg, credspec.Event{ //nolint:gosec // G101: the field holds a credential ID, not a value.
		Credential: "example-app/prod/SESSION_KEY", Type: credspec.EventCreated, At: created, By: "ops",
	}); err != nil {
		panic(err)
	}

	now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	for _, c := range reg.Credentials {
		a := credspec.Assess(c, log.Record(c.ID), now, credspec.DefaultWarn)
		fmt.Printf("%s: %s (%s)\n", c.ID, a.State, a.Reason)
	}
	// Output:
	// example-app/staging/SESSION_KEY: unrecorded (active, but no created or rotated event)
	// example-app/prod/SESSION_KEY: due (rotation due 2026-07-14)
}
