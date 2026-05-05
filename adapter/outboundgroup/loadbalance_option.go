package outboundgroup

import (
	"errors"
	"fmt"
	"time"
)

type loadBalanceTopNOption struct {
	Primary int `json:"primary" mapstructure:"primary"`
	Backup  int `json:"backup" mapstructure:"backup"`
}

type loadBalanceHashOption struct {
	KeyParts     []string `json:"key_parts" mapstructure:"key_parts"`
	VirtualNodes int      `json:"virtual_nodes" mapstructure:"virtual_nodes"`
	OnEmptyKey   string   `json:"on_empty_key" mapstructure:"on_empty_key"`
	KeySalt      string   `json:"key_salt" mapstructure:"key_salt"`
	Tolerance    int      `json:"tolerance" mapstructure:"tolerance"`
}

type loadBalanceHysteresisOption struct {
	PrimaryFailures int           `json:"primary_failures" mapstructure:"primary_failures"`
	BackupHoldTime  time.Duration `json:"backup_hold_time" mapstructure:"backup_hold_time"`
}

type LoadBalanceOption struct {
	PrimaryOutbounds []string                   `json:"primary_outbounds" mapstructure:"primary_outbounds"`
	BackupOutbounds  []string                   `json:"backup_outbounds" mapstructure:"backup_outbounds"`
	TopN             *loadBalanceTopNOption     `json:"top_n" mapstructure:"top_n"`
	Hash             *loadBalanceHashOption     `json:"hash" mapstructure:"hash"`
	Hysteresis       *loadBalanceHysteresisOption `json:"hysteresis" mapstructure:"hysteresis"`
	EmptyPoolAction          string `json:"empty_pool_action" mapstructure:"empty_pool_action"`
	InterruptExistConnections bool   `json:"interrupt_exist_connections" mapstructure:"interrupt_exist_connections"`
	PreferDomain             bool   `json:"prefer_domain" mapstructure:"prefer_domain"`
	Strategy                 string `json:"strategy" mapstructure:"strategy"`
}

var (
	errInvalidPrimaryOutbounds = errors.New("primary_outbounds must be a non-empty array of strings")
	errInvalidBackupOutbounds  = errors.New("backup_outbounds must be an array of strings")
	errInvalidTopN             = errors.New("top_n must be an object with integer primary and backup fields")
	errInvalidHash            = errors.New("hash must be an object")
	errInvalidHysteresis      = errors.New("hysteresis must be an object")
	errInvalidEmptyPoolAction = errors.New("empty_pool_action must be one of: error, select_next, wait")
	errInvalidStrategy        = errors.New("strategy must be one of: consistent-hashing, round-robin, sticky-sessions, random, consistent_hash")
)

func parseLoadBalanceOption(config map[string]any) (*LoadBalanceOption, bool, error) {
	lbOpt := &LoadBalanceOption{}

	if primaryOutbounds, ok := config["primary_outbounds"]; ok {
		primaryList, err := parseStringArrayField(primaryOutbounds, "primary_outbounds")
		if err != nil {
			return nil, false, fmt.Errorf("%s: %w", lbOpt.Strategy, errInvalidPrimaryOutbounds)
		}
		if len(primaryList) == 0 {
			return nil, false, errInvalidPrimaryOutbounds
		}
		lbOpt.PrimaryOutbounds = primaryList

		if backupOutbounds, ok := config["backup_outbounds"]; ok {
			backupList, err := parseStringArrayField(backupOutbounds, "backup_outbounds")
			if err != nil {
				return nil, false, errInvalidBackupOutbounds
			}
			lbOpt.BackupOutbounds = backupList
		}

		if topN, ok := config["top_n"]; ok {
			topNOpt, err := parseTopNOption(topN)
			if err != nil {
				return nil, false, err
			}
			lbOpt.TopN = topNOpt
		}

		if hash, ok := config["hash"]; ok {
			hashOpt, err := parseHashOption(hash)
			if err != nil {
				return nil, false, err
			}
			lbOpt.Hash = hashOpt
		}

		if hysteresis, ok := config["hysteresis"]; ok {
			hystOpt, err := parseHysteresisOption(hysteresis)
			if err != nil {
				return nil, false, err
			}
			lbOpt.Hysteresis = hystOpt
		}

		if emptyPoolAction, ok := config["empty_pool_action"]; ok {
			action, ok := emptyPoolAction.(string)
			if !ok {
				return nil, false, errInvalidEmptyPoolAction
			}
			switch action {
			case "error", "select_next", "wait":
				lbOpt.EmptyPoolAction = action
			default:
				return nil, false, errInvalidEmptyPoolAction
			}
		}

		if iec, ok := config["interrupt_exist_connections"]; ok {
			lbOpt.InterruptExistConnections, ok = iec.(bool)
			if !ok {
				lbOpt.InterruptExistConnections = false
			}
		}

		if pd, ok := config["prefer_domain"]; ok {
			lbOpt.PreferDomain, ok = pd.(bool)
			if !ok {
				lbOpt.PreferDomain = false
			}
		}

		lbOpt.Strategy = parseStrategy(config)

		return lbOpt, true, nil
	}

	return nil, false, nil
}

func parseStringArrayField(value any, fieldName string) ([]string, error) {
	list, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%s: expected array, got %T", fieldName, value)
	}
	result := make([]string, 0, len(list))
	for i, item := range list {
		str, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("%s[%d]: expected string, got %T", fieldName, i, item)
		}
		result = append(result, str)
	}
	return result, nil
}

func parseTopNOption(value any) (*loadBalanceTopNOption, error) {
	obj, ok := value.(map[string]any)
	if !ok {
		return nil, errInvalidTopN
	}

	opt := &loadBalanceTopNOption{}

	if primary, ok := obj["primary"]; ok {
		primaryVal, ok := toInt(primary)
		if !ok || primaryVal < 0 {
			return nil, fmt.Errorf("top_n.primary: expected non-negative integer, got %v", primary)
		}
		opt.Primary = primaryVal
	}

	if backup, ok := obj["backup"]; ok {
		backupVal, ok := toInt(backup)
		if !ok || backupVal < 0 {
			return nil, fmt.Errorf("top_n.backup: expected non-negative integer, got %v", backup)
		}
		opt.Backup = backupVal
	}

	return opt, nil
}

func parseHashOption(value any) (*loadBalanceHashOption, error) {
	obj, ok := value.(map[string]any)
	if !ok {
		return nil, errInvalidHash
	}

	opt := &loadBalanceHashOption{
		VirtualNodes: 100,
		OnEmptyKey:   "random",
	}

	if keyParts, ok := obj["key_parts"]; ok {
		parts, err := parseStringArrayField(keyParts, "hash.key_parts")
		if err != nil {
			return nil, err
		}
		opt.KeyParts = parts
	}

	if virtualNodes, ok := obj["virtual_nodes"]; ok {
		vn, ok := toInt(virtualNodes)
		if !ok || vn < 0 {
			return nil, fmt.Errorf("hash.virtual_nodes: expected non-negative integer, got %v", virtualNodes)
		}
		opt.VirtualNodes = vn
	}

	if onEmptyKey, ok := obj["on_empty_key"]; ok {
		onk, ok := onEmptyKey.(string)
		if !ok {
			return nil, fmt.Errorf("hash.on_empty_key: expected string, got %T", onEmptyKey)
		}
		opt.OnEmptyKey = onk
	}

	if keySalt, ok := obj["key_salt"]; ok {
		ks, ok := keySalt.(string)
		if !ok {
			return nil, fmt.Errorf("hash.key_salt: expected string, got %T", keySalt)
		}
		opt.KeySalt = ks
	}

	if tolerance, ok := obj["tolerance"]; ok {
		tol, ok := toInt(tolerance)
		if !ok || tol < 0 {
			return nil, fmt.Errorf("hash.tolerance: expected non-negative integer, got %v", tolerance)
		}
		opt.Tolerance = tol
	}

	return opt, nil
}

func parseHysteresisOption(value any) (*loadBalanceHysteresisOption, error) {
	obj, ok := value.(map[string]any)
	if !ok {
		return nil, errInvalidHysteresis
	}

	opt := &loadBalanceHysteresisOption{}

	if primaryFailures, ok := obj["primary_failures"]; ok {
		pf, ok := toInt(primaryFailures)
		if !ok || pf < 0 {
			return nil, fmt.Errorf("hysteresis.primary_failures: expected non-negative integer, got %v", primaryFailures)
		}
		opt.PrimaryFailures = pf
	}

	if backupHoldTime, ok := obj["backup_hold_time"]; ok {
		switch t := backupHoldTime.(type) {
		case string:
			d, err := time.ParseDuration(t)
			if err != nil {
				return nil, fmt.Errorf("hysteresis.backup_hold_time: invalid duration: %w", err)
			}
			opt.BackupHoldTime = d
		case int, int64, float64:
			seconds, ok := toInt(t)
			if !ok {
				return nil, fmt.Errorf("hysteresis.backup_hold_time: invalid duration value: %v", t)
			}
			opt.BackupHoldTime = time.Duration(seconds) * time.Second
		default:
			return nil, fmt.Errorf("hysteresis.backup_hold_time: expected duration string or integer seconds, got %T", backupHoldTime)
		}
	}

	return opt, nil
}

func toInt(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	default:
		return 0, false
	}
}

func ValidateLoadBalanceOption(opt *LoadBalanceOption) error {
	if len(opt.PrimaryOutbounds) == 0 {
		return errInvalidPrimaryOutbounds
	}

	switch opt.Strategy {
	case "consistent-hashing", "round-robin", "sticky-sessions", "random", "consistent_hash":
	default:
		return fmt.Errorf("%w: got '%s'", errInvalidStrategy, opt.Strategy)
	}

	if opt.EmptyPoolAction != "" {
		switch opt.EmptyPoolAction {
		case "error", "select_next", "wait":
		default:
			return errInvalidEmptyPoolAction
		}
	}

	return nil
}

func GetLoadBalanceStrategy(strategy string) string {
	switch strategy {
	case "consistent_hash":
		return "consistent-hashing"
	default:
		return strategy
	}
}