package logging

import (
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"

	"github.com/temporalio/s2s-proxy/config"
)

const (
	AdminService       = "adminService"
	WorkflowService    = "workflowService"
	ReplicationStreams = "replicationStreams"
	ShardManager       = "shardManager"
	ShardRouting       = "shardRouting"
)

type (
	LogComponentName string
	// LoggerProvider provides customized loggers for different components.
	// Based on the component name, different throttling levels can be applied.
	// Right now, any tags stored with the LoggerProvider with With() will be applied to all loggers returned by Get().
	LoggerProvider interface {
		// Get returns a logger for the given component. If there is no custom config, the root logger will be returned.
		Get(component LogComponentName) log.Logger
		// With returns a new logger provider with the given tags added to all loggers.
		With(tags ...tag.Tag) LoggerProvider
	}
	loggerProvider struct {
		root    log.Logger
		loggers map[LogComponentName]log.Logger
		tags    []tag.Tag
	}
)

var defaultLoggers = map[LogComponentName]config.LoggingConfig{
	AdminService:       {ThrottleMaxRPS: 3.0},
	ReplicationStreams: {ThrottleMaxRPS: 3.0 / 60.0},
	ShardManager:       {ThrottleMaxRPS: 3.0},
	ShardRouting:       {ThrottleMaxRPS: 3.0 / 60.0},
}

func NewLoggerProvider(root log.Logger, config config.ConfigProvider) LoggerProvider {
	logConfigs := config.GetS2SProxyConfig().LogConfigs
	throttledRootLog := log.NewThrottledLogger(root, config.GetS2SProxyConfig().Logging.GetThrottleMaxRPS)
	loggersByComponent := make(map[LogComponentName]log.Logger, max(len(logConfigs), len(defaultLoggers)))
	for component, defaultConfig := range defaultLoggers {
		loggersByComponent[component] = loggerForConfig(throttledRootLog, defaultConfig)
	}
	for component, logConfig := range logConfigs {
		loggersByComponent[LogComponentName(component)] = loggerForConfig(throttledRootLog, logConfig)
	}
	return &loggerProvider{
		root:    root,
		loggers: loggersByComponent,
	}
}

func loggerForConfig(logger log.Logger, config config.LoggingConfig) log.Logger {
	if config.Disabled {
		return log.NewNoopLogger()
	}
	return log.NewThrottledLogger(logger, config.GetThrottleMaxRPS)
}

func (l *loggerProvider) Get(component LogComponentName) log.Logger {
	logger, exists := l.loggers[component]
	if !exists {
		logger = l.root
	}
	return log.With(logger, l.tags...)
}

func (l *loggerProvider) With(tags ...tag.Tag) LoggerProvider {
	return &loggerProvider{
		root:    l.root,
		loggers: l.loggers,
		tags:    append(l.tags, tags...),
	}
}
