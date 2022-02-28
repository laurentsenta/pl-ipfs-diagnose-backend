package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/p2p/protocol/identify"
)

func main() {
	// start a libp2p node with default settings
	node, err := libp2p.New(
		libp2p.ConnectionGater(&privateAddrFilterConnectionGater{}),
	)
	if err != nil {
		panic(err)
	}

	// Identify service
	idService, err := identify.NewIDService(node, identify.UserAgent("ipfs-check"))

	daemon := &daemon{host: node, idService: idService}

	// Server
	l, err := net.Listen("tcp", ":3333")
	if err != nil {
		panic(err)
	}

	fmt.Printf("listening on %v\n", l.Addr())

	// Wait if needed

	fmt.Println("Ready to start serving")

	http.HandleFunc("/identify", func(writer http.ResponseWriter, request *http.Request) {
		out, err := daemon.runIdentify(writer, request.RequestURI)

		writer.Header().Add("Access-Control-Allow-Origin", "*")

		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = writer.Write([]byte(err.Error()))
			return
		}

		outputJSON, err := json.Marshal(out)

		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = writer.Write([]byte(err.Error()))
			return
		}

		_, err = writer.Write(outputJSON)

		if err != nil {
			fmt.Printf("could not return data over HTTP: %v\n", err.Error())
		}
	})

	http.HandleFunc("/find", func(writer http.ResponseWriter, request *http.Request) {
		out, err := daemon.runFindContent(writer, request.RequestURI)

		writer.Header().Add("Access-Control-Allow-Origin", "*")

		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = writer.Write([]byte(err.Error()))
			return
		}

		outputJSON, err := json.Marshal(out)

		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = writer.Write([]byte(err.Error()))
			return
		}

		_, err = writer.Write(outputJSON)

		if err != nil {
			fmt.Printf("could not return data over HTTP: %v\n", err.Error())
		}
	})

	err = http.Serve(l, nil)
	if err != nil {
		panic(err)
	}
}
