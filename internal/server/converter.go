package server

import (
	"fmt"

	identityv1 "github.com/agynio/identity/.gen/go/agynio/api/identity/v1"
)

func identityTypeFromProto(value identityv1.IdentityType) (int16, error) {
	switch value {
	case identityv1.IdentityType_IDENTITY_TYPE_USER:
		return 1, nil
	case identityv1.IdentityType_IDENTITY_TYPE_AGENT:
		return 2, nil
	case identityv1.IdentityType_IDENTITY_TYPE_CHANNEL:
		return 3, nil
	case identityv1.IdentityType_IDENTITY_TYPE_RUNNER:
		return 4, nil
	case identityv1.IdentityType_IDENTITY_TYPE_APP:
		return 5, nil
	case identityv1.IdentityType_IDENTITY_TYPE_UNSPECIFIED:
		return 0, fmt.Errorf("identity_type must be provided")
	default:
		return 0, fmt.Errorf("unknown identity_type: %v", value)
	}
}

func identityTypeToProto(value int16) (identityv1.IdentityType, error) {
	switch value {
	case 1:
		return identityv1.IdentityType_IDENTITY_TYPE_USER, nil
	case 2:
		return identityv1.IdentityType_IDENTITY_TYPE_AGENT, nil
	case 3:
		return identityv1.IdentityType_IDENTITY_TYPE_CHANNEL, nil
	case 4:
		return identityv1.IdentityType_IDENTITY_TYPE_RUNNER, nil
	case 5:
		return identityv1.IdentityType_IDENTITY_TYPE_APP, nil
	default:
		return identityv1.IdentityType_IDENTITY_TYPE_UNSPECIFIED, fmt.Errorf("unknown identity_type: %d", value)
	}
}
