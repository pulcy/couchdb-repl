FROM alpine:3.3

ADD ./couchdb-repl /app/

ENTRYPOINT ["/app/couchdb-repl"]
