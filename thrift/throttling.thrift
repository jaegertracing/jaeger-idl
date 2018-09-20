namespace java com.uber.jaeger.thrift.throttling

struct ThrottlingConfig {
    1: required i32 maxOperations;
    2: required double creditsPerSecond;
    3: required double maxBalance;
}

struct ServiceThrottlingConfig {
    1: required string serviceName;
    2: required ThrottlingConfig config;
}

struct ThrottlingResponse {
    1: required ThrottlingConfig defaultConfig;
    2: required list<ServiceThrottlingConfig> serviceConfigs;
}

service ThrottlingService {
    ThrottlingResponse getThrottlingConfigs(1: list<string> serviceNames);
}
