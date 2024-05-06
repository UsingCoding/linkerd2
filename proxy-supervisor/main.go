package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/linkerd/linkerd2/pkg/supervisor"
)

func main() {
	ctx := context.Background()

	err := runSupervisor(ctx)
	if err != nil {
		log.Fatal(err)
	}
}

func runSupervisor(ctx context.Context) error {
	params, logger, err := configureSupervisorParams()
	if err != nil {
		return err
	}

	useSupervisor := extractEnv("LINKERD2_PROXY_SUPERVISOR", func(s string) bool {
		b, err := strconv.ParseBool(s)
		if err != nil {
			return false
		}
		return b
	})

	s := supervisor.TransparentSupervisor
	if useSupervisor {
		s = supervisor.Supervisor
	}

	return s(
		ctx,
		params,
		logger,
	)
}

func configureSupervisorParams() (supervisor.Params, logrus.FieldLogger, error) {
	pod := extractEnv("_pod_name", func(s string) string {
		return s
	})
	if pod == "" {
		return supervisor.Params{}, nil, fmt.Errorf("_pod_name not set")
	}
	ns := extractEnv("_pod_ns", func(s string) string {
		return s
	})
	if ns == "" {
		return supervisor.Params{}, nil, fmt.Errorf("_pod_ns not set")
	}
	gracefulTimeout := extractEnv("LINKERD2_PROXY_SUPERVISOR_GRACEFULTIMEOUT", func(s string) time.Duration {
		d, _ := time.ParseDuration(s)
		return d
	})
	// zero gracefulTimeout
	if gracefulTimeout == time.Duration(0) {
		gracefulTimeout = 30 * time.Second
	}
	// use the same log format as proxy
	logFormat := extractEnv("LINKERD2_PROXY_LOG_FORMAT", func(s string) string {
		return s
	})

	timestampFormat := time.RFC3339Nano
	fieldMap := logrus.FieldMap{
		logrus.FieldKeyTime: "@timestamp",
		logrus.FieldKeyMsg:  "message",
	}

	var formatter logrus.Formatter

	switch logFormat {
	case "json":
		formatter = &logrus.JSONFormatter{
			TimestampFormat: timestampFormat,
			FieldMap:        fieldMap,
		}
	default:
		formatter = &logrus.TextFormatter{
			TimestampFormat: timestampFormat,
			FieldMap:        fieldMap,
		}
	}

	logger := logrus.New()
	logger.SetFormatter(formatter)
	fieldLogger := logger.WithFields(logrus.Fields{
		"target": "supervisor",
	})

	return supervisor.Params{
		Namespace:       ns,
		Pod:             pod,
		GracefulTimeout: gracefulTimeout,
		// run proxy-identity
		// All input args are static
		ChildProcessArgs: []string{"/usr/lib/linkerd/linkerd2-proxy-identity"},
	}, fieldLogger, nil
}

func extractEnv[T any](env string, f func(string) T) (t T) {
	v, b := os.LookupEnv(env)
	if !b {
		return t
	}
	return f(v)
}
