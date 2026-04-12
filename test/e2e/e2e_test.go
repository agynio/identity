//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	identityv1 "github.com/agynio/identity/.gen/go/agynio/api/identity/v1"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func TestIdentityServiceE2E(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	conn, err := grpc.DialContext(ctx, identityAddr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn.Close()
	})

	client := identityv1.NewIdentityServiceClient(conn)

	t.Run("RegisterIdentity", func(t *testing.T) {
		identityID := uuid.NewString()
		_, err := client.RegisterIdentity(ctx, &identityv1.RegisterIdentityRequest{
			IdentityId:   identityID,
			IdentityType: identityv1.IdentityType_IDENTITY_TYPE_USER,
		})
		require.NoError(t, err)

		_, err = client.RegisterIdentity(ctx, &identityv1.RegisterIdentityRequest{
			IdentityId:   identityID,
			IdentityType: identityv1.IdentityType_IDENTITY_TYPE_USER,
		})
		requireStatusCode(t, err, codes.AlreadyExists)

		getResp, err := client.GetIdentityType(ctx, &identityv1.GetIdentityTypeRequest{IdentityId: identityID})
		require.NoError(t, err)
		require.Equal(t, identityv1.IdentityType_IDENTITY_TYPE_USER, getResp.IdentityType)
	})

	t.Run("BatchGetIdentityTypes", func(t *testing.T) {
		firstID := uuid.NewString()
		secondID := uuid.NewString()
		_, err := client.RegisterIdentity(ctx, &identityv1.RegisterIdentityRequest{
			IdentityId:   firstID,
			IdentityType: identityv1.IdentityType_IDENTITY_TYPE_USER,
		})
		require.NoError(t, err)
		_, err = client.RegisterIdentity(ctx, &identityv1.RegisterIdentityRequest{
			IdentityId:   secondID,
			IdentityType: identityv1.IdentityType_IDENTITY_TYPE_AGENT,
		})
		require.NoError(t, err)

		batchResp, err := client.BatchGetIdentityTypes(ctx, &identityv1.BatchGetIdentityTypesRequest{
			IdentityIds: []string{secondID, uuid.NewString(), firstID},
		})
		require.NoError(t, err)
		require.Len(t, batchResp.Entries, 2)
		require.True(t, hasIdentityType(batchResp.Entries, firstID, identityv1.IdentityType_IDENTITY_TYPE_USER))
		require.True(t, hasIdentityType(batchResp.Entries, secondID, identityv1.IdentityType_IDENTITY_TYPE_AGENT))
	})

	t.Run("Nicknames", func(t *testing.T) {
		orgID := uuid.NewString()
		userID := uuid.NewString()
		agentID := uuid.NewString()
		appID := uuid.NewString()

		_, err := client.RegisterIdentity(ctx, &identityv1.RegisterIdentityRequest{
			IdentityId:   userID,
			IdentityType: identityv1.IdentityType_IDENTITY_TYPE_USER,
		})
		require.NoError(t, err)
		_, err = client.RegisterIdentity(ctx, &identityv1.RegisterIdentityRequest{
			IdentityId:   agentID,
			IdentityType: identityv1.IdentityType_IDENTITY_TYPE_AGENT,
		})
		require.NoError(t, err)
		_, err = client.RegisterIdentity(ctx, &identityv1.RegisterIdentityRequest{
			IdentityId:   appID,
			IdentityType: identityv1.IdentityType_IDENTITY_TYPE_APP,
		})
		require.NoError(t, err)

		_, err = client.SetNickname(ctx, &identityv1.SetNicknameRequest{
			OrganizationId: orgID,
			IdentityId:     userID,
			Nickname:       "alice",
		})
		require.NoError(t, err)
		_, err = client.SetNickname(ctx, &identityv1.SetNicknameRequest{
			OrganizationId: orgID,
			IdentityId:     userID,
			Nickname:       "alice-updated",
		})
		require.NoError(t, err)

		resolveResp, err := client.ResolveNickname(ctx, &identityv1.ResolveNicknameRequest{
			OrganizationId: orgID,
			Nickname:       "alice-updated",
		})
		require.NoError(t, err)
		require.Equal(t, userID, resolveResp.IdentityId)
		require.Equal(t, identityv1.IdentityType_IDENTITY_TYPE_USER, resolveResp.IdentityType)
		require.Nil(t, resolveResp.InstallationId)

		_, err = client.ResolveNickname(ctx, &identityv1.ResolveNicknameRequest{
			OrganizationId: orgID,
			Nickname:       "alice",
		})
		requireStatusCode(t, err, codes.NotFound)

		_, err = client.SetNickname(ctx, &identityv1.SetNicknameRequest{
			OrganizationId: orgID,
			IdentityId:     agentID,
			Nickname:       "alice-updated",
		})
		requireStatusCode(t, err, codes.AlreadyExists)

		installationOne := uuid.NewString()
		_, err = client.SetNickname(ctx, &identityv1.SetNicknameRequest{
			OrganizationId: orgID,
			IdentityId:     appID,
			Nickname:       "app-main",
			InstallationId: &installationOne,
		})
		require.NoError(t, err)
		installationTwo := uuid.NewString()
		_, err = client.SetNickname(ctx, &identityv1.SetNicknameRequest{
			OrganizationId: orgID,
			IdentityId:     appID,
			Nickname:       "app-secondary",
			InstallationId: &installationTwo,
		})
		require.NoError(t, err)

		appResolve, err := client.ResolveNickname(ctx, &identityv1.ResolveNicknameRequest{
			OrganizationId: orgID,
			Nickname:       "app-main",
		})
		require.NoError(t, err)
		require.Equal(t, appID, appResolve.IdentityId)
		require.Equal(t, identityv1.IdentityType_IDENTITY_TYPE_APP, appResolve.IdentityType)
		require.NotNil(t, appResolve.InstallationId)
		require.Equal(t, installationOne, appResolve.GetInstallationId())

		_, err = client.RemoveNickname(ctx, &identityv1.RemoveNicknameRequest{
			OrganizationId: orgID,
			IdentityId:     userID,
		})
		require.NoError(t, err)
		_, err = client.ResolveNickname(ctx, &identityv1.ResolveNicknameRequest{
			OrganizationId: orgID,
			Nickname:       "alice-updated",
		})
		requireStatusCode(t, err, codes.NotFound)
	})

	t.Run("NegativePaths", func(t *testing.T) {
		_, err := client.GetIdentityType(ctx, &identityv1.GetIdentityTypeRequest{IdentityId: uuid.NewString()})
		requireStatusCode(t, err, codes.NotFound)

		_, err = client.GetIdentityType(ctx, &identityv1.GetIdentityTypeRequest{IdentityId: "not-a-uuid"})
		requireStatusCode(t, err, codes.InvalidArgument)

		_, err = client.RegisterIdentity(ctx, &identityv1.RegisterIdentityRequest{IdentityId: uuid.NewString()})
		requireStatusCode(t, err, codes.InvalidArgument)

		_, err = client.RegisterIdentity(ctx, &identityv1.RegisterIdentityRequest{IdentityId: "bad", IdentityType: identityv1.IdentityType_IDENTITY_TYPE_USER})
		requireStatusCode(t, err, codes.InvalidArgument)

		_, err = client.BatchGetIdentityTypes(ctx, &identityv1.BatchGetIdentityTypesRequest{IdentityIds: []string{"bad"}})
		requireStatusCode(t, err, codes.InvalidArgument)
	})
}

func hasIdentityType(entries []*identityv1.IdentityTypeEntry, id string, identityType identityv1.IdentityType) bool {
	for _, entry := range entries {
		if entry.GetIdentityId() == id && entry.GetIdentityType() == identityType {
			return true
		}
	}
	return false
}

func requireStatusCode(t *testing.T, err error, code codes.Code) {
	t.Helper()
	statusErr, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, code, statusErr.Code())
}
