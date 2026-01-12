package constant

import "time"

// AnyTLSSessionManagement config for global session pool management
type AnyTLSSessionManagement struct {
	EnsureIdleSession           int           `yaml:"ensure-idle-session"`             // Proactive pool size (target)
	MinIdleSession              int           `yaml:"min-idle-session"`                // Passive idle protection
	MinIdleSessionForAge        int           `yaml:"min-idle-session-for-age"`        // Passive age protection
	EnsureIdleSessionCreateRate int           `yaml:"ensure-idle-session-create-rate"` // Max new sessions per cycle
	MaxConnectionLifetime       time.Duration `yaml:"max-connection-lifetime"`         // Age-based rotation
	ConnectionLifetimeJitter    time.Duration `yaml:"connection-lifetime-jitter"`      // Randomization range
	IdleSessionTimeout          time.Duration `yaml:"idle-session-timeout"`            // Idle timeout
	IdleSessionCheckInterval    time.Duration `yaml:"idle-session-check-interval"`     // Cleanup cycle interval
}
