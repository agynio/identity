package server

import (
	"fmt"

	identityv1 "github.com/agynio/identity/.gen/go/agynio/api/identity/v1"
)

const (
	dbIdentityTypeUser    int16 = 1
	dbIdentityTypeAgent   int16 = 2
	dbIdentityTypeChannel int16 = 3
	dbIdentityTypeRunner  int16 = 4
	dbIdentityTypeApp     int16 = 5
)

var identityTypeMappings = []struct {
	proto identityv1.IdentityType
	db    int16
}{
	{proto: identityv1.IdentityType_IDENTITY_TYPE_USER, db: dbIdentityTypeUser},
	{proto: identityv1.IdentityType_IDENTITY_TYPE_AGENT, db: dbIdentityTypeAgent},
	{proto: identityv1.IdentityType_IDENTITY_TYPE_CHANNEL, db: dbIdentityTypeChannel},
	{proto: identityv1.IdentityType_IDENTITY_TYPE_RUNNER, db: dbIdentityTypeRunner},
	{proto: identityv1.IdentityType_IDENTITY_TYPE_APP, db: dbIdentityTypeApp},
}

func identityTypeFromProto(value identityv1.IdentityType) (int16, error) {
	if value == identityv1.IdentityType_IDENTITY_TYPE_UNSPECIFIED {
		return 0, fmt.Errorf("identity_type must be provided")
	}
	for _, mapping := range identityTypeMappings {
		if mapping.proto == value {
			return mapping.db, nil
		}
	}
	return 0, fmt.Errorf("unknown identity_type: %v", value)
}

func identityTypeToProto(value int16) (identityv1.IdentityType, error) {
	for _, mapping := range identityTypeMappings {
		if mapping.db == value {
			return mapping.proto, nil
		}
	}
	var zero identityv1.IdentityType
	return zero, fmt.Errorf("unknown identity_type: %d", value)
}
