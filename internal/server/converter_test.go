package server

import (
	"testing"

	identityv1 "github.com/agynio/identity/.gen/go/agynio/api/identity/v1"
	"github.com/stretchr/testify/require"
)

func TestIdentityTypeRoundTrip(t *testing.T) {
	testCases := []struct {
		name  string
		proto identityv1.IdentityType
		db    int16
	}{
		{name: "user", proto: identityv1.IdentityType_IDENTITY_TYPE_USER, db: 1},
		{name: "agent", proto: identityv1.IdentityType_IDENTITY_TYPE_AGENT, db: 2},
		{name: "runner", proto: identityv1.IdentityType_IDENTITY_TYPE_RUNNER, db: 4},
		{name: "app", proto: identityv1.IdentityType_IDENTITY_TYPE_APP, db: 5},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			dbValue, err := identityTypeFromProto(testCase.proto)
			require.NoError(t, err)
			require.Equal(t, testCase.db, dbValue)

			protoValue, err := identityTypeToProto(dbValue)
			require.NoError(t, err)
			require.Equal(t, testCase.proto, protoValue)
		})
	}
}

func TestIdentityTypeFromProtoRejectsUnspecified(t *testing.T) {
	_, err := identityTypeFromProto(identityv1.IdentityType_IDENTITY_TYPE_UNSPECIFIED)
	require.Error(t, err)
}

func TestIdentityTypeFromProtoRejectsUnknown(t *testing.T) {
	_, err := identityTypeFromProto(identityv1.IdentityType(99))
	require.Error(t, err)
}

func TestIdentityTypeToProtoRejectsUnknown(t *testing.T) {
	_, err := identityTypeToProto(99)
	require.Error(t, err)
}
