package blockchain

import (
	"fmt"
	"github.com/hyperledger/fabric-sdk-go/pkg/client/channel"
	"github.com/hyperledger/fabric-sdk-go/pkg/client/event"
	"github.com/hyperledger/fabric-sdk-go/pkg/client/ledger"
	mspclient "github.com/hyperledger/fabric-sdk-go/pkg/client/msp"
	"github.com/hyperledger/fabric-sdk-go/pkg/client/resmgmt"
	"github.com/hyperledger/fabric-sdk-go/pkg/common/errors/retry"
	"github.com/hyperledger/fabric-sdk-go/pkg/common/providers/msp"
	"github.com/hyperledger/fabric-sdk-go/pkg/core/config"
	packager "github.com/hyperledger/fabric-sdk-go/pkg/fab/ccpackager/gopackager"
	"github.com/hyperledger/fabric-sdk-go/pkg/fabsdk"
	"github.com/hyperledger/fabric-sdk-go/third_party/github.com/hyperledger/fabric/common/cauthdsl"
	"github.com/pkg/errors"
)

// FabricSetup implementation
type FabricSetup struct {
	ConfigFile      string
	OrgID           string
	OrdererID       string
	ChannelID       []string
	ChainCodeID     []string
	initialized     bool
	ChannelConfig   []string
	ChaincodeGoPath string
	ChaincodePath   []string
	OrgAdmin        string
	OrgName         string
	UserName        string
	client          []*channel.Client //1.chainhero 溯源 2.token
	ledger          []*ledger.Client  //1.chainhero 溯源 2.token
	admin           []*resmgmt.Client //1.chainhero 溯源 2.token
	sdk             *fabsdk.FabricSDK //1.chainhero 溯源 2.token
	event           []*event.Client   //1.chainhero 溯源 2.token
}

// Initialize reads the configuration file and sets up the client, chain and event hub
func (setup *FabricSetup) Initialize() error {

	setup.client = make([]*channel.Client, len(setup.ChannelID))
	setup.ledger = make([]*ledger.Client, len(setup.ChannelID))
	setup.admin = make([]*resmgmt.Client, len(setup.ChannelID))
	setup.event = make([]*event.Client, len(setup.ChannelID))
	// Add parameters for the initialization
	if setup.initialized {
		return errors.New("sdk already initialized")
	}

	// Initialize the SDK with the configuration file
	sdk, err := fabsdk.New(config.FromFile(setup.ConfigFile))
	if err != nil {
		return errors.WithMessage(err, "failed to create SDK")
	}
	setup.sdk = sdk
	fmt.Println("SDK created")

	// The resource management client is responsible for managing channels (create/update channel)
	resourceManagerClientContext := setup.sdk.Context(fabsdk.WithUser(setup.OrgAdmin), fabsdk.WithOrg(setup.OrgName))
	if err != nil {
		return errors.WithMessage(err, "failed to load Admin identity")
	}
	resMgmtClient, err := resmgmt.New(resourceManagerClientContext)
	if err != nil {
		return errors.WithMessage(err, "failed to create channel management client from Admin identity")
	}
	fmt.Println("Ressource management client created")

	// The MSP client allow us to retrieve user information from their identity, like its signing identity which we will need to save the channel
	mspClient, err := mspclient.New(sdk.Context(), mspclient.WithOrg(setup.OrgName))
	if err != nil {
		return errors.WithMessage(err, "failed to create MSP client")
	}
	adminIdentity, err := mspClient.GetSigningIdentity(setup.OrgAdmin)
	if err != nil {
		return errors.WithMessage(err, "failed to get admin signing identity")
	}

	//循环创建通道 加入通道
	for i, _ := range setup.ChannelID {
		setup.admin[i] = resMgmtClient
		req := resmgmt.SaveChannelRequest{ChannelID: setup.ChannelID[i], ChannelConfigPath: setup.ChannelConfig[i], SigningIdentities: []msp.SigningIdentity{adminIdentity}}
		txID, err := setup.admin[i].SaveChannel(req, resmgmt.WithOrdererEndpoint(setup.OrdererID))
		if err != nil || txID.TransactionID == "" {
			return errors.WithMessage(err, "failed to save channel")
		}
		//Make admin user join the previously created channel
		if err = setup.admin[i].JoinChannel(setup.ChannelID[i], resmgmt.WithRetry(retry.DefaultResMgmtOpts), resmgmt.WithOrdererEndpoint(setup.OrdererID)); err != nil {
			return errors.WithMessage(err, "failed to make admin join channel")
		}
		fmt.Println(setup.ChannelID[i])
		fmt.Println("Channel created")
		fmt.Println("Channel joined")
	}

	fmt.Println("Initialization Successful")
	setup.initialized = true
	return nil
}

func (setup *FabricSetup) InstallAndInstantiateCC() error {

	for i := 0; i < len(setup.ChainCodeID); i++ {
		fmt.Println("链码操作")
		fmt.Println(setup.ChainCodeID[i])
		ccPkg, err := packager.NewCCPackage(setup.ChaincodePath[i], setup.ChaincodeGoPath)
		if err != nil {
			return errors.WithMessage(err, "failed to create chaincode package")
		}

		// Install example cc to org peers
		installCCReq := resmgmt.InstallCCRequest{Name: setup.ChainCodeID[i], Path: setup.ChaincodePath[i], Version: "0", Package: ccPkg}
		_, err = setup.admin[i].InstallCC(installCCReq, resmgmt.WithRetry(retry.DefaultResMgmtOpts))
		if err != nil {
			return errors.WithMessage(err, "failed to install chaincode")
		}
		fmt.Println("Chaincode installed")

		// Set up chaincode policy
		ccPolicy := cauthdsl.SignedByAnyMember([]string{"org1.hf.chainhero.io"})

		resp, err := setup.admin[i].InstantiateCC(setup.ChannelID[i], resmgmt.InstantiateCCRequest{Name: setup.ChainCodeID[i], Path: setup.ChaincodePath[i], Version: "0", Args: [][]byte{[]byte("init")}, Policy: ccPolicy})
		if err != nil || resp.TransactionID == "" {
			return errors.WithMessage(err, "failed to instantiate the chaincode")
		}
		fmt.Println("Chaincode instantiated")

		// Channel client is used to query and execute transactions
		clientContext := setup.sdk.ChannelContext(setup.ChannelID[i], fabsdk.WithUser(setup.UserName))
		setup.client[i], err = channel.New(clientContext)
		if err != nil {
			return errors.WithMessage(err, "failed to create new channel client")
		}
		fmt.Println("Channel client created")

		//A、准备通道上下文
		//B、创建分类帐客户端
		org1AdminChannelContext := setup.sdk.ChannelContext(setup.ChannelID[i], fabsdk.WithUser(setup.OrgAdmin), fabsdk.WithOrg(setup.OrgName))

		// Ledger client
		setup.ledger[i], err = ledger.New(org1AdminChannelContext)
		if err != nil {
			return errors.WithMessage(err, "Failed to create new resource management client: %s")
		}
		if err != nil {
			return errors.WithMessage(err, "Failed to create new resource management client: %s")
		}

		// Creation of the client which will enables access to our channel events
		setup.event[i], err = event.New(clientContext)
		if err != nil {
			return errors.WithMessage(err, "failed to create new event client")
		}
		fmt.Println("Event client created")
	}

	fmt.Println("Chaincode Installation & Instantiation Successful")
	return nil
}

func (setup *FabricSetup) CloseSDK() {
	setup.sdk.Close()
}


// 下方数据持久化  再次启动使用

func (setup *FabricSetup) Initialize_buk() error {

	// Add parameters for the initialization
	if setup.initialized {
		return errors.New("sdk already initialized")
	}

	// Initialize the SDK with the configuration file
	sdk, err := fabsdk.New(config.FromFile(setup.ConfigFile))
	if err != nil {
		return errors.WithMessage(err, "failed to create SDK")
	}
	setup.sdk = sdk
	fmt.Println("SDK created")

	// The resource management client is responsible for managing channels (create/update channel)
	resourceManagerClientContext := setup.sdk.Context(fabsdk.WithUser(setup.OrgAdmin), fabsdk.WithOrg(setup.OrgName))
	if err != nil {
		return errors.WithMessage(err, "failed to load Admin identity")
	}
	resMgmtClient, err := resmgmt.New(resourceManagerClientContext)
	if err != nil {
		return errors.WithMessage(err, "failed to create channel management client from Admin identity")
	}
	for i := 0; i < len(setup.ChannelID); i++ {
		setup.admin[i] = resMgmtClient
	}
	fmt.Println("Initialization Successful")
	setup.initialized = true
	return nil
}

func (setup *FabricSetup) InstallAndInstantiateCC_buk() error {
	for i := 0; i < len(setup.ChainCodeID); i++ {
		fmt.Println("链码操作")
		fmt.Println(setup.ChainCodeID[i])

		var err error
		// Channel client is used to query and execute transactions
		clientContext := setup.sdk.ChannelContext(setup.ChannelID[i], fabsdk.WithUser(setup.UserName))
		setup.client[i], err = channel.New(clientContext)
		if err != nil {
			return errors.WithMessage(err, "failed to create new channel client")
		}
		fmt.Println("Channel client created")

		//A、准备通道上下文
		//B、创建分类帐客户端
		org1AdminChannelContext := setup.sdk.ChannelContext(setup.ChannelID[i], fabsdk.WithUser(setup.OrgAdmin), fabsdk.WithOrg(setup.OrgName))

		// Ledger client
		setup.ledger[i], err = ledger.New(org1AdminChannelContext)
		if err != nil {
			return errors.WithMessage(err, "Failed to create new resource management client: %s")
		}
		if err != nil {
			return errors.WithMessage(err, "Failed to create new resource management client: %s")
		}

		// Creation of the client which will enables access to our channel events
		setup.event[i], err = event.New(clientContext)
		if err != nil {
			return errors.WithMessage(err, "failed to create new event client")
		}
		fmt.Println("Event client created")
	}

	fmt.Println("Chaincode Installation & Instantiation Successful")
	return nil
}
