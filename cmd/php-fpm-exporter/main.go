package main

import (
	"fmt"
	"os"

	exporter "github.com/bakins/php-fpm-exporter"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	addr         *string
	endpoint     *string
	fcgiEndpoint *string
)

func serverCmd(cmd *cobra.Command, args []string) {

	logger, err := exporter.NewLogger()
	if err != nil {
		panic(err)
	}

	e, err := exporter.New(
		exporter.SetAddress(*addr),
		exporter.SetEndpoint(*endpoint),
		exporter.SetFastcgi(*fcgiEndpoint),
		exporter.SetLogger(logger),
	)

	if err != nil {
		logger.Fatal("failed to create exporter", zap.Error(err))
	}

	if err := e.Run(); err != nil {
		logger.Fatal("failed to run exporter", zap.Error(err))
	}
}

var rootCmd = &cobra.Command{
	Use:   "php-fpm-exporter",
	Short: "php-fpm metrics exporter",
	Run:   serverCmd,
}

func main() {
	addr = rootCmd.PersistentFlags().StringP("addr", "", "127.0.0.1:8080", "listen address for metrics handler")
	endpoint = rootCmd.PersistentFlags().StringP("endpoint", "", "http://127.0.0.1:9000/status", "url for php-fpm status")
	fcgiEndpoint = rootCmd.PersistentFlags().String("fastcgi", "", "fastcgi url. If this is set, fastcgi will be used instead of HTTP")

	if err := rootCmd.Execute(); err != nil {
		fmt.Printf("root command failed: %v", err)
		os.Exit(-2)
	}
}
