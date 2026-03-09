package admin

import (
	"encoding/json"
	"fmt"
)

func marshalAdminStateData(stateName string, data interface{}) ([]byte, error) {
	if data == nil {
		return nil, nil
	}

	switch stateName {
	case StateAwaitingPassword:
		return nil, nil
	case StateAssignRoleSelect, StateChangeRoleSelect:
		v, ok := data.(*UserPickerData)
		if !ok {
			return nil, fmt.Errorf("unexpected admin state payload for %s", stateName)
		}
		return json.Marshal(v)
	case StateAssignRoleText, StateChangeRoleText:
		v, ok := data.(*RoleInputData)
		if !ok {
			return nil, fmt.Errorf("unexpected admin state payload for %s", stateName)
		}
		return json.Marshal(v)
	case StateBalanceAdjustMode, StateBalanceAdjustPicker, StateBalanceAdjustAmount, StateBalanceAdjustConfirm, StateBalanceDeltaName, StateBalanceDeltaAmount:
		v, ok := data.(*BalanceAdjustData)
		if !ok {
			return nil, fmt.Errorf("unexpected admin state payload for %s", stateName)
		}
		return json.Marshal(v)
	default:
		return nil, fmt.Errorf("unsupported admin state %s", stateName)
	}
}

func unmarshalAdminStateData(stateName string, raw []byte) (interface{}, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	switch stateName {
	case StateAssignRoleSelect, StateChangeRoleSelect:
		var v UserPickerData
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, err
		}
		return &v, nil
	case StateAssignRoleText, StateChangeRoleText:
		var v RoleInputData
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, err
		}
		return &v, nil
	case StateBalanceAdjustMode, StateBalanceAdjustPicker, StateBalanceAdjustAmount, StateBalanceAdjustConfirm, StateBalanceDeltaName, StateBalanceDeltaAmount:
		var v BalanceAdjustData
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, err
		}
		return &v, nil
	case StateAwaitingPassword:
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported admin state %s", stateName)
	}
}
