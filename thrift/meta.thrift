namespace java com.uber.health

enum State {
	REFUSING = 0,
	ACCEPTING = 1,
	DRAINING = 2,
	STOPPING = 3,
	DRAIN_WAITING = 4,
}

struct HealthStatus {
	1: required bool ok
	2: optional string message
	3: optional State state
}

service Meta {
	HealthStatus health()
}
