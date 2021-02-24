package testing

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/datachainlab/ibc-solidity/pkg/chains"
	"github.com/datachainlab/ibc-solidity/pkg/contract"
	"github.com/datachainlab/ibc-solidity/pkg/contract/ibcchannel"
	"github.com/datachainlab/ibc-solidity/pkg/contract/ibcclient"
	"github.com/datachainlab/ibc-solidity/pkg/contract/ibcconnection"
	"github.com/datachainlab/ibc-solidity/pkg/contract/ibcroutingmodule"
	"github.com/datachainlab/ibc-solidity/pkg/contract/ibcstore"
	"github.com/datachainlab/ibc-solidity/pkg/contract/ibft2client"
	"github.com/datachainlab/ibc-solidity/pkg/contract/simpletokenmodule"
	channeltypes "github.com/datachainlab/ibc-solidity/pkg/ibc/channel"
	clienttypes "github.com/datachainlab/ibc-solidity/pkg/ibc/client"
	"github.com/gogo/protobuf/proto"

	"github.com/datachainlab/ibc-solidity/pkg/wallet"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
)

const (
	BesuIBFT2Client              = "ibft2"
	DefaultChannelVersion        = "ics20-1"
	DefaultDelayPeriod    uint64 = 0
	DefaultPrefix                = "ibc"
	TransferPort                 = "transfer"
)

type Chain struct {
	t *testing.T

	// Core Modules
	client           contract.Client
	IBCClient        ibcclient.Ibcclient
	IBCConnection    ibcconnection.Ibcconnection
	IBCChannel       ibcchannel.Ibcchannel
	IBCRoutingModule ibcroutingmodule.Ibcroutingmodule
	IBFT2Client      ibft2client.Ibft2client
	IBCStore         ibcstore.Ibcstore

	// App Modules
	SimpletokenModule simpletokenmodule.Simpletokenmodule

	chainID int64

	ContractConfig ContractConfig

	key0 *ecdsa.PrivateKey

	// State
	LastContractState *contract.ContractState

	// IBC specific helpers
	ClientIDs   []string          // ClientID's used on this chain
	Connections []*TestConnection // track connectionID's created for this chain
	IBCID       uint64
}

type ContractConfig interface {
	GetIBCStoreAddress() common.Address
	GetIBCClientAddress() common.Address
	GetIBCConnectionAddress() common.Address
	GetIBCChannelAddress() common.Address
	GetIBCRoutingModuleAddress() common.Address
	GetIBFT2ClientAddress() common.Address
	GetSimpleTokenModuleAddress() common.Address
}

func NewChain(t *testing.T, chainID int64, client contract.Client, config ContractConfig, mnemonicPhrase string, ibcID uint64) *Chain {
	ibft2Client, err := ibft2client.NewIbft2client(config.GetIBFT2ClientAddress(), client)
	if err != nil {
		t.Error(err)
	}
	ibcStore, err := ibcstore.NewIbcstore(config.GetIBCStoreAddress(), client)
	if err != nil {
		t.Error(err)
	}
	ibcClient, err := ibcclient.NewIbcclient(config.GetIBCClientAddress(), client)
	if err != nil {
		t.Error(err)
	}
	ibcConnection, err := ibcconnection.NewIbcconnection(config.GetIBCConnectionAddress(), client)
	if err != nil {
		t.Error(err)
	}
	ibcChannel, err := ibcchannel.NewIbcchannel(config.GetIBCChannelAddress(), client)
	if err != nil {
		t.Error(err)
	}
	ibcRoutingModule, err := ibcroutingmodule.NewIbcroutingmodule(config.GetIBCRoutingModuleAddress(), client)
	if err != nil {
		t.Error(err)
	}

	simpletokenModule, err := simpletokenmodule.NewSimpletokenmodule(config.GetSimpleTokenModuleAddress(), client)
	if err != nil {
		t.Error(err)
	}

	key0, err := wallet.GetPrvKeyFromMnemonicAndHDWPath(mnemonicPhrase, "m/44'/60'/0'/0/0")
	if err != nil {
		t.Error(err)
	}

	return &Chain{t: t, client: client, IBFT2Client: *ibft2Client, IBCStore: *ibcStore, IBCClient: *ibcClient, IBCConnection: *ibcConnection, IBCChannel: *ibcChannel, IBCRoutingModule: *ibcRoutingModule, SimpletokenModule: *simpletokenModule, chainID: chainID, ContractConfig: config, key0: key0, IBCID: ibcID}
}

func (chain *Chain) Client() contract.Client {
	return chain.client
}

func (chain *Chain) TxOpts(ctx context.Context) *bind.TransactOpts {
	return contract.MakeGenTxOpts(big.NewInt(chain.chainID), chain.key0)(ctx)
}

func (chain *Chain) CallOpts(ctx context.Context) *bind.CallOpts {
	opts := chain.TxOpts(ctx)
	return &bind.CallOpts{
		From:    opts.From,
		Context: opts.Context,
	}
}

func (chain *Chain) ChainID() int64 {
	return chain.chainID
}

func (chain *Chain) ChainIDString() string {
	return fmt.Sprint(chain.chainID)
}

func (chain *Chain) GetCommitmentPrefix() []byte {
	return []byte(DefaultPrefix)
}

func (chain *Chain) GetClientState(clientID string) *clienttypes.ClientState {
	ctx := context.Background()
	cs, err := chain.IBFT2Client.GetClientState(chain.CallOpts(ctx), clientID)
	require.NoError(chain.t, err)
	return &clienttypes.ClientState{
		ChainId:         cs.ChainId,
		IbcStoreAddress: cs.IbcStoreAddress,
		LatestHeight:    cs.LatestHeight,
	}
}

func (chain *Chain) GetContractState(counterparty *Chain, counterpartyClientID string, storageKeys [][]byte) (*contract.ContractState, error) {
	height := counterparty.GetClientState(counterpartyClientID).LatestHeight
	return chain.client.GetContractState(
		context.Background(),
		chain.ContractConfig.GetIBCStoreAddress(),
		storageKeys,
		big.NewInt(int64(height)),
	)
}

func (chain *Chain) Init() error {
	ctx := context.Background()
	if err := chain.WaitIfNoError(ctx)(
		chain.IBCStore.SetIBCModule(
			chain.TxOpts(ctx),
			chain.ContractConfig.GetIBCClientAddress(),
			chain.ContractConfig.GetIBCConnectionAddress(),
			chain.ContractConfig.GetIBCChannelAddress(),
			chain.ContractConfig.GetIBCRoutingModuleAddress(),
		),
	); err != nil {
		return err
	}

	if err := chain.WaitIfNoError(ctx)(
		chain.IBCClient.RegisterClient(
			chain.TxOpts(ctx),
			BesuIBFT2Client,
			chain.ContractConfig.GetIBFT2ClientAddress(),
		),
	); err != nil {
		return err
	}

	if err := chain.WaitIfNoError(ctx)(
		chain.IBCChannel.SetIBCModule(
			chain.TxOpts(ctx),
			chain.ContractConfig.GetIBCRoutingModuleAddress(),
		),
	); err != nil {
		return err
	}

	return nil
}

func (chain *Chain) ConstructMsgCreateClient(counterparty *Chain, clientID string) ibcclient.IBCMsgsMsgCreateClient {
	clientState := clienttypes.ClientState{
		ChainId:         counterparty.ChainIDString(),
		IbcStoreAddress: counterparty.ContractConfig.GetIBCStoreAddress().Bytes(),
		LatestHeight:    counterparty.LastHeader().Base.Number.Uint64(),
	}
	consensusState := clienttypes.ConsensusState{
		Timestamp:  counterparty.LastHeader().Base.Time,
		Root:       counterparty.LastHeader().Base.Root.Bytes(),
		Validators: counterparty.LastValidators(),
	}
	clientStateBytes, err := proto.Marshal(&clientState)
	if err != nil {
		panic(err)
	}
	consensusStateBytes, err := proto.Marshal(&consensusState)
	if err != nil {
		panic(err)
	}
	return ibcclient.IBCMsgsMsgCreateClient{
		ClientId:            clientID,
		ClientType:          BesuIBFT2Client,
		Height:              clientState.LatestHeight,
		ClientStateBytes:    clientStateBytes,
		ConsensusStateBytes: consensusStateBytes,
	}
}

func (chain *Chain) ConstructMsgUpdateClient(counterparty *Chain, clientID string) ibcclient.IBCMsgsMsgUpdateClient {
	trustedHeight := chain.GetClientState(clientID).LatestHeight
	var header = clienttypes.Header{
		BesuHeaderRlp:     counterparty.LastContractState.SealingHeaderRLP(),
		Seals:             counterparty.LastContractState.CommitSeals,
		TrustedHeight:     trustedHeight,
		AccountStateProof: counterparty.LastContractState.AccountProofRLP(),
	}
	headerBytes, err := proto.Marshal(&header)
	if err != nil {
		panic(err)
	}
	return ibcclient.IBCMsgsMsgUpdateClient{
		ClientId: clientID,
		Header:   headerBytes,
	}
}

func (chain *Chain) UpdateHeader() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for {
		state, err := chain.client.GetContractState(ctx, chain.ContractConfig.GetIBCStoreAddress(), nil, nil)
		if err != nil {
			panic(err)
		}
		if chain.LastContractState == nil || state.ParsedHeader.Base.Number.Cmp(chain.LastHeader().Base.Number) == 1 {
			chain.LastContractState = state
			return
		} else {
			continue
		}
	}
}

func (chain *Chain) CreateBesuClient(ctx context.Context, counterparty *Chain, clientID string) error {
	msg := chain.ConstructMsgCreateClient(counterparty, clientID)
	return chain.WaitIfNoError(ctx)(
		chain.IBCClient.CreateClient(chain.TxOpts(ctx), msg),
	)
}

func (chain *Chain) UpdateBesuClient(ctx context.Context, counterparty *Chain, clientID string) error {
	msg := chain.ConstructMsgUpdateClient(counterparty, clientID)
	return chain.WaitIfNoError(ctx)(
		chain.IBCClient.UpdateClient(chain.TxOpts(ctx), msg),
	)
}

func (chain *Chain) ConnectionOpenInit(ctx context.Context, counterparty *Chain, connection, counterpartyConnection *TestConnection) error {
	return chain.WaitIfNoError(ctx)(
		chain.IBCConnection.ConnectionOpenInit(
			chain.TxOpts(ctx),
			ibcconnection.IBCMsgsMsgConnectionOpenInit{
				ClientId:     connection.ClientID,
				ConnectionId: connection.ID,
				Counterparty: ibcconnection.CounterpartyData{
					ClientId:     connection.CounterpartyClientID,
					ConnectionId: "",
					Prefix:       ibcconnection.MerklePrefixData{KeyPrefix: counterparty.GetCommitmentPrefix()},
				},
				DelayPeriod: DefaultDelayPeriod,
			},
		),
	)
}

func (chain *Chain) ConnectionOpenTry(ctx context.Context, counterparty *Chain, connection, counterpartyConnection *TestConnection) error {
	proof, err := counterparty.QueryProof(chain, connection.ClientID, chain.ConnectionStateCommitmentSlot(counterpartyConnection.ID))
	if err != nil {
		return err
	}
	return chain.WaitIfNoError(ctx)(
		chain.IBCConnection.ConnectionOpenTry(
			chain.TxOpts(ctx),
			ibcconnection.IBCMsgsMsgConnectionOpenTry{
				ConnectionId: connection.ID,
				Counterparty: ibcconnection.CounterpartyData{
					ClientId:     counterpartyConnection.ClientID,
					ConnectionId: counterpartyConnection.ID,
					Prefix:       ibcconnection.MerklePrefixData{KeyPrefix: counterparty.GetCommitmentPrefix()},
				},
				DelayPeriod: DefaultDelayPeriod,
				ClientId:    connection.ClientID,
				// ClientState: ibcconnection.ClientStateData{}, // TODO set chain's clientState
				CounterpartyVersions: []ibcconnection.VersionData{
					{Identifier: "1", Features: []string{"ORDER_ORDERED", "ORDER_UNORDERED"}},
				},
				ProofHeight: proof.Height,
				ProofInit:   proof.Data,
			},
		),
	)
}

// ConnectionOpenAck will construct and execute a MsgConnectionOpenAck.
func (chain *Chain) ConnectionOpenAck(
	ctx context.Context,
	counterparty *Chain,
	connection, counterpartyConnection *TestConnection,
) error {
	proof, err := counterparty.QueryProof(chain, connection.ClientID, chain.ConnectionStateCommitmentSlot(counterpartyConnection.ID))
	if err != nil {
		return err
	}
	return chain.WaitIfNoError(ctx)(
		chain.IBCConnection.ConnectionOpenAck(
			chain.TxOpts(ctx),
			ibcconnection.IBCMsgsMsgConnectionOpenAck{
				ConnectionId:             connection.ID,
				CounterpartyConnectionID: counterpartyConnection.ID,
				// clientState
				Version:     ibcconnection.VersionData{Identifier: "1", Features: []string{"ORDER_ORDERED", "ORDER_UNORDERED"}},
				ProofTry:    proof.Data,
				ProofHeight: proof.Height,
			},
		),
	)
}

func (chain *Chain) ConnectionOpenConfirm(
	ctx context.Context,
	counterparty *Chain,
	connection, counterpartyConnection *TestConnection,
) error {
	proof, err := counterparty.QueryProof(chain, connection.ClientID, chain.ConnectionStateCommitmentSlot(counterpartyConnection.ID))
	if err != nil {
		return err
	}
	return chain.WaitIfNoError(ctx)(
		chain.IBCConnection.ConnectionOpenConfirm(
			chain.TxOpts(ctx),
			ibcconnection.IBCMsgsMsgConnectionOpenConfirm{
				ConnectionId: connection.ID,
				ProofAck:     proof.Data,
				ProofHeight:  proof.Height,
			},
		),
	)
}

func (chain *Chain) ChannelOpenInit(
	ctx context.Context,
	ch, counterparty TestChannel,
	order channeltypes.Channel_Order,
	connectionID string,
) error {
	return chain.WaitIfNoError(ctx)(
		chain.IBCChannel.ChannelOpenInit(
			chain.TxOpts(ctx),
			ibcchannel.IBCMsgsMsgChannelOpenInit{
				ChannelId: ch.ID,
				PortId:    ch.PortID,
				Channel: ibcchannel.ChannelData{
					State:    uint8(channeltypes.INIT),
					Ordering: uint8(order),
					Counterparty: ibcchannel.ChannelCounterpartyData{
						PortId:    counterparty.PortID,
						ChannelId: "",
					},
					ConnectionHops: []string{connectionID},
					Version:        ch.Version,
				},
			},
		),
	)
}

func (chain *Chain) ChannelOpenTry(
	ctx context.Context,
	counterparty *Chain,
	ch, counterpartyCh TestChannel,
	order channeltypes.Channel_Order,
	connectionID string,
) error {
	proof, err := counterparty.QueryProof(chain, ch.ClientID, chain.ChannelStateCommitmentSlot(counterpartyCh.PortID, counterpartyCh.ID))
	if err != nil {
		return err
	}
	return chain.WaitIfNoError(ctx)(
		chain.IBCChannel.ChannelOpenTry(
			chain.TxOpts(ctx),
			ibcchannel.IBCMsgsMsgChannelOpenTry{
				PortId:    ch.PortID,
				ChannelId: ch.ID,
				Channel: ibcchannel.ChannelData{
					State:    uint8(channeltypes.TRYOPEN),
					Ordering: uint8(order),
					Counterparty: ibcchannel.ChannelCounterpartyData{
						PortId:    counterpartyCh.PortID,
						ChannelId: counterpartyCh.ID,
					},
					ConnectionHops: []string{connectionID},
					Version:        ch.Version,
				},
				CounterpartyVersion: counterpartyCh.Version,
				ProofInit:           proof.Data,
				ProofHeight:         proof.Height,
			},
		),
	)
}

func (chain *Chain) ChannelOpenAck(
	ctx context.Context,
	counterparty *Chain,
	ch, counterpartyCh TestChannel,
) error {
	proof, err := counterparty.QueryProof(chain, ch.ClientID, chain.ChannelStateCommitmentSlot(counterpartyCh.PortID, counterpartyCh.ID))
	if err != nil {
		return err
	}
	return chain.WaitIfNoError(ctx)(
		chain.IBCChannel.ChannelOpenAck(
			chain.TxOpts(ctx),
			ibcchannel.IBCMsgsMsgChannelOpenAck{
				PortId:                ch.PortID,
				ChannelId:             ch.ID,
				CounterpartyVersion:   counterpartyCh.Version,
				CounterpartyChannelId: counterpartyCh.ID,
				ProofTry:              proof.Data,
				ProofHeight:           proof.Height,
			},
		),
	)
}

func (chain *Chain) ChannelOpenConfirm(
	ctx context.Context,
	counterparty *Chain,
	ch, counterpartyCh TestChannel,
) error {
	proof, err := counterparty.QueryProof(chain, ch.ClientID, chain.ChannelStateCommitmentSlot(counterpartyCh.PortID, counterpartyCh.ID))
	if err != nil {
		return err
	}
	return chain.WaitIfNoError(ctx)(
		chain.IBCChannel.ChannelOpenConfirm(
			chain.TxOpts(ctx),
			ibcchannel.IBCMsgsMsgChannelOpenConfirm{
				PortId:      ch.PortID,
				ChannelId:   ch.ID,
				ProofAck:    proof.Data,
				ProofHeight: proof.Height,
			},
		),
	)
}

func (chain *Chain) SendPacket(
	ctx context.Context,
	packet channeltypes.Packet,
) error {
	return chain.WaitIfNoError(ctx)(
		chain.IBCChannel.SendPacket(
			chain.TxOpts(ctx),
			packetToCallData(packet),
		),
	)
}

func (chain *Chain) RecvPacket(
	ctx context.Context,
	counterparty *Chain,
	ch, counterpartyCh TestChannel,
	packet channeltypes.Packet,
) error {
	proof, err := counterparty.QueryProof(chain, ch.ClientID, chain.PacketCommitmentSlot(packet.SourcePort, packet.SourceChannel, packet.Sequence))
	if err != nil {
		return err
	}
	return chain.WaitIfNoError(ctx)(
		chain.IBCChannel.RecvPacket(
			chain.TxOpts(ctx),
			ibcchannel.IBCMsgsMsgPacketRecv{
				Packet:      packetToCallData(packet),
				Proof:       proof.Data,
				ProofHeight: proof.Height,
			},
		),
	)
}

func (chain *Chain) HandlePacketRecv(
	ctx context.Context,
	counterparty *Chain,
	ch, counterpartyCh TestChannel,
	packet channeltypes.Packet,
) error {
	proof, err := counterparty.QueryProof(chain, ch.ClientID, chain.PacketCommitmentSlot(packet.SourcePort, packet.SourceChannel, packet.Sequence))
	if err != nil {
		return err
	}
	return chain.WaitIfNoError(ctx)(
		chain.IBCRoutingModule.RecvPacket(
			chain.TxOpts(ctx),
			ibcroutingmodule.IBCMsgsMsgPacketRecv{
				Packet: ibcroutingmodule.PacketData{
					Sequence:           packet.Sequence,
					SourcePort:         packet.SourcePort,
					SourceChannel:      packet.SourceChannel,
					DestinationPort:    packet.DestinationPort,
					DestinationChannel: packet.DestinationChannel,
					Data:               packet.Data,
					TimeoutHeight:      ibcroutingmodule.HeightData(packet.TimeoutHeight),
					TimeoutTimestamp:   packet.TimeoutTimestamp,
				},
				Proof:       proof.Data,
				ProofHeight: proof.Height,
			},
		),
	)
}

func (chain *Chain) HandlePacketAcknowledgement(
	ctx context.Context,
	counterparty *Chain,
	ch, counterpartyCh TestChannel,
	packet channeltypes.Packet,
	acknowledgement []byte,
) error {
	proof, err := counterparty.QueryProof(chain, ch.ClientID, chain.PacketAcknowledgementCommitmentSlot(packet.DestinationPort, packet.DestinationChannel, packet.Sequence))
	if err != nil {
		return err
	}
	return chain.WaitIfNoError(ctx)(
		chain.IBCRoutingModule.AcknowledgePacket(
			chain.TxOpts(ctx),
			ibcroutingmodule.IBCMsgsMsgPacketAcknowledgement{
				Packet: ibcroutingmodule.PacketData{
					Sequence:           packet.Sequence,
					SourcePort:         packet.SourcePort,
					SourceChannel:      packet.SourceChannel,
					DestinationPort:    packet.DestinationPort,
					DestinationChannel: packet.DestinationChannel,
					Data:               packet.Data,
					TimeoutHeight:      ibcroutingmodule.HeightData(packet.TimeoutHeight),
					TimeoutTimestamp:   packet.TimeoutTimestamp,
				},
				Acknowledgement: acknowledgement,
				Proof:           proof.Data,
				ProofHeight:     proof.Height,
			},
		),
	)
}

func packetToCallData(packet channeltypes.Packet) ibcchannel.PacketData {
	return ibcchannel.PacketData{
		Sequence:           packet.Sequence,
		SourcePort:         packet.SourcePort,
		SourceChannel:      packet.SourceChannel,
		DestinationPort:    packet.DestinationPort,
		DestinationChannel: packet.DestinationChannel,
		Data:               packet.Data,
		TimeoutHeight:      ibcchannel.HeightData(packet.TimeoutHeight),
		TimeoutTimestamp:   packet.TimeoutTimestamp,
	}
}

// Slot calculator

func (chain *Chain) ConnectionStateCommitmentSlot(connectionID string) string {
	key, err := chain.IBCStore.ConnectionCommitmentSlot(chain.CallOpts(context.Background()), connectionID)
	require.NoError(chain.t, err)
	return "0x" + hex.EncodeToString(key[:])
}

func (chain *Chain) ChannelStateCommitmentSlot(portID, channelID string) string {
	key, err := chain.IBCStore.ChannelCommitmentSlot(chain.CallOpts(context.Background()), portID, channelID)
	require.NoError(chain.t, err)
	return "0x" + hex.EncodeToString(key[:])
}

func (chain *Chain) PacketCommitmentSlot(portID, channelID string, sequence uint64) string {
	key, err := chain.IBCStore.PacketCommitmentSlot(chain.CallOpts(context.Background()), portID, channelID, sequence)
	require.NoError(chain.t, err)
	return "0x" + hex.EncodeToString(key[:])
}

func (chain *Chain) PacketAcknowledgementCommitmentSlot(portID, channelID string, sequence uint64) string {
	key, err := chain.IBCStore.PacketAcknowledgementCommitmentSlot(chain.CallOpts(context.Background()), portID, channelID, sequence)
	require.NoError(chain.t, err)
	return "0x" + hex.EncodeToString(key[:])
}

// Querier

type Proof struct {
	Height uint64
	Data   []byte
}

func (chain *Chain) QueryProof(counterparty *Chain, counterpartyClientID string, storageKey string) (*Proof, error) {
	if !strings.HasPrefix(storageKey, "0x") {
		return nil, fmt.Errorf("storageKey must be hex string")
	}
	s, err := chain.GetContractState(counterparty, counterpartyClientID, [][]byte{[]byte(storageKey)})
	if err != nil {
		return nil, err
	}
	return &Proof{Height: s.ParsedHeader.Base.Number.Uint64(), Data: s.StorageProofRLP(0)}, nil
}

func (chain *Chain) LastValidators() [][]byte {
	var addrs [][]byte
	for _, val := range chain.LastContractState.ParsedHeader.Validators {
		addrs = append(addrs, val.Bytes())
	}
	return addrs
}

func (chain *Chain) LastHeader() *chains.ParsedHeader {
	return chain.LastContractState.ParsedHeader
}

func (chain *Chain) WaitForReceiptAndGet(ctx context.Context, tx *gethtypes.Transaction) error {
	rc, err := chain.Client().WaitForReceiptAndGet(ctx, tx)
	if err != nil {
		return err
	}
	if rc.Status == 1 {
		return nil
	} else {
		return fmt.Errorf("failed to call transaction: %v %v", err, rc)
	}
}

func (chain *Chain) WaitIfNoError(ctx context.Context) func(tx *gethtypes.Transaction, err error) error {
	return func(tx *gethtypes.Transaction, err error) error {
		if err != nil {
			return err
		}
		if err := chain.WaitForReceiptAndGet(ctx, tx); err != nil {
			return err
		}
		return nil
	}
}

// NewClientID appends a new clientID string in the format:
// ClientFor<counterparty-chain-id><index>
func (chain *Chain) NewClientID(clientType string) string {
	clientID := fmt.Sprintf("%s-%s-%v-%v", clientType, strconv.Itoa(len(chain.ClientIDs)), chain.chainID, chain.IBCID)
	chain.ClientIDs = append(chain.ClientIDs, clientID)
	return clientID
}

// AddTestConnection appends a new TestConnection which contains references
// to the connection id, client id and counterparty client id.
func (chain *Chain) AddTestConnection(clientID, counterpartyClientID string) *TestConnection {
	conn := chain.ConstructNextTestConnection(clientID, counterpartyClientID)

	chain.Connections = append(chain.Connections, conn)
	return conn
}

// ConstructNextTestConnection constructs the next test connection to be
// created given a clientID and counterparty clientID. The connection id
// format: <chainID>-conn<index>
func (chain *Chain) ConstructNextTestConnection(clientID, counterpartyClientID string) *TestConnection {
	connectionID := fmt.Sprintf("connection-%v-%v-%v", uint64(len(chain.Connections)), chain.chainID, chain.IBCID)
	return &TestConnection{
		ID:                   connectionID,
		ClientID:             clientID,
		NextChannelVersion:   DefaultChannelVersion,
		CounterpartyClientID: counterpartyClientID,
	}
}

// AddTestChannel appends a new TestChannel which contains references to the port and channel ID
// used for channel creation and interaction. See 'NextTestChannel' for channel ID naming format.
func (chain *Chain) AddTestChannel(conn *TestConnection, portID string) TestChannel {
	channel := chain.NextTestChannel(conn, portID)
	conn.Channels = append(conn.Channels, channel)
	return channel
}

// NextTestChannel returns the next test channel to be created on this connection, but does not
// add it to the list of created channels. This function is expected to be used when the caller
// has not created the associated channel in app state, but would still like to refer to the
// non-existent channel usually to test for its non-existence.
//
// channel ID format: <connectionid>-chan<channel-index>
//
// The port is passed in by the caller.
func (chain *Chain) NextTestChannel(conn *TestConnection, portID string) TestChannel {
	channelID := fmt.Sprintf("channel-%v-%v", chain.chainID, chain.IBCID)
	return TestChannel{
		PortID:               portID,
		ID:                   channelID,
		ClientID:             conn.ClientID,
		CounterpartyClientID: conn.CounterpartyClientID,
		Version:              conn.NextChannelVersion,
	}
}
