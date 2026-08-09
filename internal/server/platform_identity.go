package server

import (
	"context"
	"errors"
	"fmt"
	"time"

	authorizationv1 "github.com/agynio/identity/.gen/go/agynio/api/authorization/v1"
	"github.com/agynio/identity/internal/store"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The platform admin identity is the one identity nothing mints: it comes from
// configuration, because the controller that provisions everything else
// authenticates as it and so cannot create it. This service owns both halves of
// it -- the record and the cluster admin relation that makes it an admin -- and
// converges them at its own startup.
//
// Deliberately not a request. Granting on RegisterIdentity would let any caller
// that can reach this service hand itself cluster admin by naming the platform
// type, and every service calls RegisterIdentity. Reading the id from
// configuration means the grant names exactly one identity and nothing can ask
// for it.
type PlatformIdentity struct {
	store               identityStore
	authorizationClient authorizationChecker
}

func NewPlatformIdentity(store identityStore, authorizationClient authorizationChecker) *PlatformIdentity {
	return &PlatformIdentity{store: store, authorizationClient: authorizationClient}
}

// Ensure registers the identity and grants it admin on cluster:global. Safe to
// repeat: a record that already has the platform type is left alone, and
// writing a tuple that exists is not an error. Repeating is the point -- a
// tuple lost out from under the platform is repaired on the next start rather
// than by an operator.
func (p *PlatformIdentity) Ensure(ctx context.Context, identityID uuid.UUID) error {
	if err := p.ensureRecord(ctx, identityID); err != nil {
		return err
	}
	return p.grantClusterAdmin(ctx, identityID)
}

func (p *PlatformIdentity) ensureRecord(ctx context.Context, identityID uuid.UUID) error {
	err := p.store.RegisterIdentity(ctx, identityID, dbIdentityTypePlatform)
	if err == nil {
		return nil
	}
	var exists *store.AlreadyExistsError
	if !errors.As(err, &exists) {
		return fmt.Errorf("register: %w", err)
	}

	// Installs that predate the platform type carry this identity as a user,
	// and nothing else would ever correct the row. Only widen from that type:
	// anything else under this id belongs to someone, and adopting a runner
	// would hand it cluster admin.
	current, err := p.store.GetIdentityType(ctx, identityID)
	if err != nil {
		return fmt.Errorf("read the existing type: %w", err)
	}
	switch current {
	case dbIdentityTypePlatform:
		return nil
	case dbIdentityTypeUser:
		return p.store.SetIdentityType(ctx, identityID, dbIdentityTypePlatform)
	default:
		return fmt.Errorf("identity %s is registered as type %d, so it is not this platform's admin identity", identityID, current)
	}
}

func (p *PlatformIdentity) grantClusterAdmin(ctx context.Context, identityID uuid.UUID) error {
	_, err := p.authorizationClient.Write(ctx, &authorizationv1.WriteRequest{
		Writes: []*authorizationv1.TupleKey{{
			User:     identityObjectPrefix + identityID.String(),
			Relation: adminRelation,
			Object:   clusterObject,
		}},
	})
	if err != nil && status.Code(err) == codes.AlreadyExists {
		return nil
	}
	if err != nil {
		return fmt.Errorf("grant cluster admin: %w", err)
	}
	return nil
}

// EnsureWithRetry converges the platform identity in the background.
//
// Retried rather than attempted once because the authorization service and this
// service start together, and a platform with no administrator of its own is
// not a state anything else recovers from. This is convergence of a fact this
// service owns, not a provisioning step: nothing waits on it, and every other
// request is served meanwhile.
func (p *PlatformIdentity) EnsureWithRetry(ctx context.Context, identityID uuid.UUID, log func(string, ...any)) {
	const (
		initialBackoff = 2 * time.Second
		maxBackoff     = 30 * time.Second
		callTimeout    = 10 * time.Second
	)

	for backoff := initialBackoff; ; {
		callCtx, cancel := context.WithTimeout(ctx, callTimeout)
		err := p.Ensure(callCtx, identityID)
		cancel()
		if err == nil {
			log("platform identity %s registered and granted cluster admin", identityID)
			return
		}
		log("platform identity %s not ready (%v); retrying in %s", identityID, err, backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff *= 2; backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}
