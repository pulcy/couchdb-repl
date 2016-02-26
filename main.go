package main

import (
	"fmt"
	"net/url"
	"os"

	"github.com/op/go-logging"
	"github.com/spf13/cobra"

	"github.com/pulcy/couchdb-repl/service"
)

var (
	projectName    = "couchdb-repl"
	projectVersion = "dev"
	projectBuild   = "dev"
)

var (
	cmdMain = cobra.Command{
		Run: cmdMainRun,
	}
	appFlags struct {
		service.ServiceConfig
		serverURLs []string
	}
)

func init() {
	defaultAdminCouchDBUser := os.Getenv("COUCHDB_ADMIN_USERNAME")
	defaultAdminCouchDBPassword := os.Getenv("COUCHDB_ADMIN_PASSWORD")
	defaultReplicatorCouchDBUser := os.Getenv("COUCHDB_REPLICATOR_USERNAME")
	defaultReplicatorCouchDBPassword := os.Getenv("COUCHDB_REPLICATOR_PASSWORD")
	defaultEditorCouchDBUser := os.Getenv("COUCHDB_USERNAME")
	defaultEditorCouchDBPassword := os.Getenv("COUCHDB_PASSWORD")
	cmdMain.Flags().StringVar(&appFlags.AdminUser.UserName, "admin-user", defaultAdminCouchDBUser, "Admin user of databases")
	cmdMain.Flags().StringVar(&appFlags.AdminUser.Password, "admin-password", defaultAdminCouchDBPassword, "Admin password of databases")
	cmdMain.Flags().StringVar(&appFlags.EditorUser.UserName, "editor-user", defaultEditorCouchDBUser, "Editor user of databases")
	cmdMain.Flags().StringVar(&appFlags.EditorUser.Password, "editor-password", defaultEditorCouchDBPassword, "Editor password of databases")
	cmdMain.Flags().StringVar(&appFlags.ReplicatorUser.UserName, "replicator-user", defaultReplicatorCouchDBUser, "Replicator user of databases")
	cmdMain.Flags().StringVar(&appFlags.ReplicatorUser.Password, "replicator-password", defaultReplicatorCouchDBPassword, "Replicator password of databases")
	cmdMain.Flags().StringSliceVar(&appFlags.serverURLs, "server-url", nil, "URLs of the servers to configure")
	cmdMain.Flags().StringSliceVar(&appFlags.DatabaseNames, "db", nil, "Names of a database to replicate")
}

func main() {
	cmdMain.Execute()
}

func cmdMainRun(cmd *cobra.Command, args []string) {
	logger := logging.MustGetLogger(projectName)

	// Validate arguments
	assertArgIsSet(appFlags.AdminUser.UserName, "--admin-user")
	assertArgIsSet(appFlags.AdminUser.Password, "--admin-password")
	assertArgIsSet(appFlags.EditorUser.UserName, "--editor-user")
	assertArgIsSet(appFlags.EditorUser.Password, "--editor-password")
	assertArgIsSet(appFlags.ReplicatorUser.UserName, "--replicator-user")
	assertArgIsSet(appFlags.ReplicatorUser.Password, "--replicator-password")
	if len(appFlags.serverURLs) == 0 {
		Exitf("--server-url must be set\n")
	}
	if len(appFlags.DatabaseNames) == 0 {
		Exitf("--db must be set\n")
	}

	// Parse URLs
	for _, serverURL := range appFlags.serverURLs {
		couchUrl, err := url.Parse(serverURL)
		if err != nil {
			Exitf("Failed to parse server-url '%s': %#v", serverURL, err)
		}
		appFlags.ServiceConfig.ServerURLs = append(appFlags.ServiceConfig.ServerURLs, *couchUrl)
	}

	// Setup service
	service := service.NewService(appFlags.ServiceConfig, service.ServiceDependencies{
		Logger: logger,
	})

	// Log version
	logger.Infof("Starting %s, version %s build %s", projectName, projectVersion, projectBuild)

	// Running replication setup
	if err := service.Run(); err != nil {
		Exitf("Replication setup failed: %s\n", err.Error())
	}
	logger.Info("Replication setup succeeded")

	// We're done
}

func showUsage(cmd *cobra.Command, args []string) {
	cmd.Usage()
}

func Exitf(format string, args ...interface{}) {
	fmt.Printf(format, args...)
	fmt.Println()
	os.Exit(1)
}

func assertArgIsSet(arg, argKey string) {
	if arg == "" {
		Exitf("%s must be set\n", argKey)
	}
}
