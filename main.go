package main

import (
	"bufio"
	"fmt"
	"github.com/chainHero/heroes-service/blockchain"
	"github.com/chainHero/heroes-service/web"
	"github.com/chainHero/heroes-service/web/controllers"
	"io"
	"os"
	"strings"
)

func main() {
	
	// Definition of the Fabric SDK properties    多通道配置，先生成对应通道配置文件，交易配置文件， 修改  channelid  ChannelConfig  ChainCodeID  ChaincodePath   以及config.yaml文件
	fSetup := blockchain.FabricSetup{
		// Network parameters
		OrdererID: "orderer.hf.chainhero.io",

		// Channel parameters channel   1.chainhero 溯源 2.token
		ChannelID: []string{"chainhero", "token"},
		ChannelConfig: []string{os.Getenv("GOPATH") + "/src/github.com/chainHero/heroes-service/fixtures/artifacts/chainhero.channel.tx",
			os.Getenv("GOPATH") + "/src/github.com/chainHero/heroes-service/fixtures/artifacts/token.channel.tx"},

		// Chaincode parameters   ChainCode   1.chainhero 溯源 2.token
		ChainCodeID:     []string{"heroes-service", "token-service"},
		ChaincodeGoPath: os.Getenv("GOPATH"),
		ChaincodePath:   []string{"github.com/chainHero/heroes-service/chaincode/", "github.com/chainHero/heroes-service/chaincodetoken/"},

		OrgAdmin:   "Admin",
		OrgName:    "org1",
		ConfigFile: "config.yaml",

		// User parameters
		UserName: "User1",
	}

	// Initialization of the Fabric SDK from the previously set properties
	err := fSetup.Initialize()
	if err != nil {
		fmt.Printf("Unable to initialize the Fabric SDK: %v\n", err)
		return
	}
	// Close SDK
	defer fSetup.CloseSDK()

	// Install and instantiate the chaincode
	err = fSetup.InstallAndInstantiateCC()
	if err != nil {
		fmt.Printf("Unable to install and instantiate the chaincode: %v\n", err)
		return
	}
}
