package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/42wim/matterbridge/bridge/config"
	"github.com/42wim/matterbridge/gateway"
	"github.com/42wim/matterbridge/gateway/bridgemap"
	"github.com/google/gops/agent"
	prefixed "github.com/matterbridge/logrus-prefixed-formatter"
	"github.com/sirupsen/logrus"
	"github.com/bmatsuo/lmdb-go/lmdb"
)

var (
	version = "1.15.2-dev"
	githash string

	flagConfig  = flag.String("conf", "matterbridge.toml", "config file")
	flagDebug   = flag.Bool("debug", false, "enable debug")
	flagVersion = flag.Bool("version", false, "show version")
	flagGops    = flag.Bool("gops", false, "enable gops agent")
)

func main() {
	flag.Parse()
	if *flagVersion {
		fmt.Printf("version: %s %s\n", version, githash)
		return
	}

	rootLogger := setupLogger()
	logger := rootLogger.WithFields(logrus.Fields{"prefix": "main"})

	if *flagGops {
		if err := agent.Listen(agent.Options{}); err != nil {
			logger.Errorf("Failed to start gops agent: %#v", err)
		} else {
			defer agent.Close()
		}
	}

	logger.Printf("Running version %s %s", version, githash)
	if strings.Contains(version, "-dev") {
		logger.Println("WARNING: THIS IS A DEVELOPMENT VERSION. Things may break.")
	}

	cfg := config.NewConfig(rootLogger, *flagConfig)
	cfg.BridgeValues().General.Debug = *flagDebug

	if cfg.BridgeValues().General.UseImgur && cfg.BridgeValues().General.ImgurLMDBPath != "" {
		var err error
		gateway.ImgurLMDBEnv, err = lmdb.NewEnv()
		if err != nil {
			logger.Fatalf("Failed to create LMDB environment")
		}
		var mapSize int64 = 2 * 1024 * 1024 * 1024
		if cfg.BridgeValues().General.ImgurLMDBSize != 0 {
			mapSize = cfg.BridgeValues().General.ImgurLMDBSize
		}
		gateway.ImgurLMDBEnv.SetMapSize(mapSize)
		if err != nil {
			logger.Fatalf("Failed to set mapping size of LMDB environment")
		}
		gateway.ImgurLMDBEnv.SetMaxDBs(1)
		if err != nil {
			logger.Fatalf("Failed to set maximum number of DBs of LMDB environment")
		}
		err = os.Mkdir(cfg.BridgeValues().General.ImgurLMDBPath, 0755)
		if err != nil && !os.IsExist(err) {
			logger.Fatalf("Failed to mkdir for LMDB environment: %s", cfg.BridgeValues().General.ImgurLMDBPath)
		}
		err = gateway.ImgurLMDBEnv.Open(cfg.BridgeValues().General.ImgurLMDBPath, lmdb.Create, 0644)
		if err != nil {
			logger.Fatalf("Failed to open LMDB environment: %s", cfg.BridgeValues().General.ImgurLMDBPath)
		}
		err = gateway.ImgurLMDBEnv.Update(func(txn *lmdb.Txn) (err error) {
			gateway.ImgurLMDBDBI, err = txn.OpenDBI("ID_Deletehash", lmdb.Create)
			if err != nil {
				return err
			}

			return nil
		})
		if err != nil {
			logger.Fatalf("Failed to open database: %s", "ID_Deletehash")
		}
		defer gateway.ImgurLMDBEnv.Close()
	}

	r, err := gateway.NewRouter(rootLogger, cfg, bridgemap.FullMap)
	if err != nil {
		logger.Fatalf("Starting gateway failed: %s", err)
	}
	if err = r.Start(); err != nil {
		logger.Fatalf("Starting gateway failed: %s", err)
	}
	logger.Printf("Gateway(s) started succesfully. Now relaying messages")
	select {}
}

func setupLogger() *logrus.Logger {
	logger := &logrus.Logger{
		Out: os.Stdout,
		Formatter: &prefixed.TextFormatter{
			PrefixPadding: 13,
			DisableColors: true,
			FullTimestamp: true,
		},
		Level: logrus.InfoLevel,
	}
	if *flagDebug || os.Getenv("DEBUG") == "1" {
		logger.Formatter = &prefixed.TextFormatter{
			PrefixPadding:   13,
			DisableColors:   true,
			FullTimestamp:   false,
			ForceFormatting: true,
		}
		logger.Level = logrus.DebugLevel
		logger.WithFields(logrus.Fields{"prefix": "main"}).Info("Enabling debug logging.")
	}
	return logger
}
