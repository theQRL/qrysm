// This binary is a simple rest API endpoint to calculate
// the QNR value of a node given its private key,ip address and port.
package main

import (
	"encoding/hex"
	"flag"
	"net"

	"github.com/libp2p/go-libp2p/core/crypto"
	log "github.com/sirupsen/logrus"
	"github.com/theQRL/go-qrl/p2p/qnode"
	"github.com/theQRL/go-qrl/p2p/qnr"
	ecdsaqrysm "github.com/theQRL/qrysm/crypto/ecdsa"
	"github.com/theQRL/qrysm/io/file"
	_ "github.com/theQRL/qrysm/runtime/maxprocs"
)

var (
	privateKey = flag.String("private", "", "Hex encoded Private key to use for calculation of QNR")
	udpPort    = flag.Int("udp-port", 0, "UDP Port to use for calculation of QNR")
	tcpPort    = flag.Int("tcp-port", 0, "TCP Port to use for calculation of QNR")
	ipAddr     = flag.String("ipAddress", "", "IP to use in calculation of QNR")
	outfile    = flag.String("out", "", "Filepath to write QNR")
)

func main() {
	flag.Parse()

	if *privateKey == "" {
		log.Fatal("No private key given")
	}
	dst, err := hex.DecodeString(*privateKey)
	if err != nil {
		panic(err)
	}
	unmarshalledKey, err := crypto.UnmarshalSecp256k1PrivateKey(dst)
	if err != nil {
		panic(err)
	}
	ecdsaPrivKey, err := ecdsaqrysm.ConvertFromInterfacePrivKey(unmarshalledKey)
	if err != nil {
		panic(err)
	}

	if net.ParseIP(*ipAddr).To4() == nil {
		log.WithField("address", *ipAddr).Fatal("Invalid ipv4 address given")
	}

	if *udpPort == 0 {
		log.WithField("port", *udpPort).Fatal("Invalid udp port given")
		return
	}

	db, err := qnode.OpenDB("")
	if err != nil {
		log.WithError(err).Fatal("Could not open node's peer database")
		return
	}
	defer db.Close()

	localNode := qnode.NewLocalNode(db, ecdsaPrivKey)
	ipEntry := qnr.IP(net.ParseIP(*ipAddr))
	udpEntry := qnr.UDP(*udpPort)
	localNode.Set(ipEntry)
	localNode.Set(udpEntry)
	if *tcpPort != 0 {
		tcpEntry := qnr.TCP(*tcpPort)
		localNode.Set(tcpEntry)
	}
	log.Info(localNode.Node().String())

	if *outfile != "" {
		err := file.WriteFile(*outfile, []byte(localNode.Node().String()))
		if err != nil {
			panic(err)
		}
		log.Infof("Wrote to %s", *outfile)
	}
}
