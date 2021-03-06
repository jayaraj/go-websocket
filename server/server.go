package server

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	rotatelogs "github.com/lestrrat/go-file-rotatelogs"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"golang.org/x/sync/errgroup"
)

type Server struct {
	context            context.Context
	shutdownFn         context.CancelFunc
	childRoutines      *errgroup.Group
	shutdownReason     string
	shutdownInProgress bool
	homepath           string
	configpath         string
}

func init() {
	log.SetLevel(log.DebugLevel)
	log.SetOutput(os.Stdout)
}

func NewServer(homepath string, configpath string) *Server {

	loadConfigurations(homepath, configpath)
	rootCtx, shutdownFn := context.WithCancel(context.Background())
	childRoutines, childCtx := errgroup.WithContext(rootCtx)

	return &Server{
		context:       childCtx,
		shutdownFn:    shutdownFn,
		childRoutines: childRoutines,
		homepath:      homepath,
		configpath:    configpath,
	}
}

func loadConfigurations(homepath string, configpath string) {
	viper.AutomaticEnv()
	viper.SetEnvPrefix("ws")
	viper.AddConfigPath(filepath.Dir(configpath))
	viper.SetConfigName(filepath.Base(configpath))
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.SetConfigType("yaml")
	logDir := homepath + "/logs"
	err := os.MkdirAll(logDir, os.ModePerm)
	if err == nil || os.IsExist(err) {
		logPath := logDir + "/log.UTC."
		writer, err := rotatelogs.New(
			fmt.Sprintf("%s.%s", logPath, "%Y-%m-%d.%H:%M"),
			rotatelogs.WithLinkName("current"),
			rotatelogs.WithMaxAge(time.Hour*48),
			rotatelogs.WithRotationTime(time.Hour*24),
		)
		if err == nil {
			mulitWriter := io.MultiWriter(os.Stdout, writer)
			log.SetOutput(mulitWriter)
		}
	}
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Fatalf("configuration file not found %s", err.Error())
		} else {
			log.Fatalf("configuration file found with error : %s", err.Error())
		}
	}
	viper.SetDefault("mode", "dev")
	if viper.Get("mode") == "prod" {
		log.SetLevel(log.ErrorLevel)
	}
	for _, key := range viper.AllKeys() {
		val := viper.Get(key)
		viper.Set(key, val)
	}

}

func (server *Server) Run() (err error) {

	services := GetServices()

	for _, service := range services {
		if err := service.Instance.Init(); err != nil {
			log.WithField("Error", err).Fatal("Starting services")
			return err
		}
	}

	for _, svc := range services {
		service, ok := svc.Instance.(BackgroundService)
		if !ok {
			continue
		}

		descriptor := svc
		server.childRoutines.Go(func() error {
			if server.shutdownInProgress {
				return nil
			}

			err := service.Run(server.context)
			server.shutdownInProgress = true
			if err != nil {
				log.WithField("reason", err.Error()).Errorf("Stopped  %s", descriptor.Name)
				return err
			}
			return nil
		})
	}

	defer func() {
		log.Debug("Waiting on services...")
		if waitErr := server.childRoutines.Wait(); waitErr != nil && reflect.TypeOf(waitErr) != reflect.TypeOf(context.Canceled) {
			log.WithField("Error", waitErr).Error("A Service failed")
			if err == nil {
				err = waitErr
			}
		}
	}()

	return
}

func (server *Server) Shutdown(reason string) {

	log.WithField("Reason", reason).Info("Shutdown started")
	server.shutdownReason = reason
	server.shutdownInProgress = true
	server.shutdownFn()

	if err := server.childRoutines.Wait(); err != nil && reflect.TypeOf(err) != reflect.TypeOf(context.Canceled) {
		log.WithField("Error", err).Error("Failed waiting for services to shutdown")
	}
}

func (server *Server) ExitCode(reason error) int {

	code := 1
	if reason == context.Canceled && server.shutdownReason != "" {
		code = 0
	} else {
		server.shutdownReason = "No Services to listen"
	}
	log.WithField("Reason", server.shutdownReason).Error("Server shutdown")
	return code
}
