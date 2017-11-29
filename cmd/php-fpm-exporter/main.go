package main

import (
        "fmt"
        "os"
        "github.com/spf13/cobra"
        "go.uber.org/zap"
)

var (
        addr     *string
        confpath *string
)

func serverCmd(cmd *cobra.Command, args []string) {

        logger, err := exporter.NewLogger()
        if err != nil {
                panic(err)
        }

        e, err := exporter.New(
                exporter.SetAddress(*addr),
                exporter.SetLogger(logger),
                exporter.SetConfPath(*confpath),
        )

        if err != nil {
                logger.Fatal("failed to create exporter", zap.Error(err))
        }

        if err := e.Run(); err != nil {
                logger.Fatal("failed to run exporter", zap.Error(err))
        }
}

var rootCmd = &cobra.Command{
        Use:   "exporter",
        Short: "metrics exporter",
        Run:   serverCmd,
}

func main() {
        addr = rootCmd.PersistentFlags().StringP("listen", "", "127.0.0.1:8080", "listen address for metrics handler")
        confpath = rootCmd.PersistentFlags().StringP("endpoints.conf", "", "", "path to config file")

        if err := rootCmd.Execute(); err != nil {
                fmt.Printf("root command failed: %v", err)
                os.Exit(-2)
        }
}
