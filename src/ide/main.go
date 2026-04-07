package ide

import (
	"fmt"
	"log"
	"net/http"
)

func Main() {
	hub := NewHub()
	sessions := NewSessionStore()
	collab := NewCollabServer(hub)
	server := NewServer(hub, sessions, collab)

	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	port := "8080"
	fmt.Printf("GoCollab IDE running at http://localhost:%s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
