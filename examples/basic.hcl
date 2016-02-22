job "basic_example" {

    task "couchdb" {
        count = 2
        image = "frodenas/couchdb"
        env {
            COUCHDB_USERNAME="some-username"
            COUCHDB_PASSWORD="some-password"
            COUCHDB_DBNAME="exampledb"
        }
        private-frontend {
            port = 5984
            register-instance = true
        }
    }

    task "couchdb-repl" {
        type = "oneshot"
        image = "pulcy/couchdb-repl:latest"
        links = [
            "basic_example.couchdb.couchdb@1",
            "basic_example.couchdb.couchdb@2"
        ]
        args = [
            "--user=some-username",
            "--password=some-password",
            "--db=exampledb",
            "--server-url={{link_url "basic_example.couchdb.couchdb@1"}}",
            "--server-url={{link_url "basic_example.couchdb.couchdb@2"}}",
        ]
    }
}