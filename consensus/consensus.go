// Copyright 2017 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// Package consensus implements different Ethereum consensus engines.
package consensus

import (
	"encoding/json"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/mapprotocol/atlas/chains"
	"github.com/mapprotocol/atlas/chains/ethereum"
	"github.com/mapprotocol/atlas/consensus/istanbul"
	"github.com/mapprotocol/atlas/core/state"
	"github.com/mapprotocol/atlas/core/types"
	"github.com/mapprotocol/atlas/core/vm"
	"github.com/mapprotocol/atlas/p2p"
	"github.com/mapprotocol/atlas/params"
	params2 "github.com/mapprotocol/atlas/params"
)

// ChainHeaderReader defines a small collection of methods needed to access the local
// blockchain during header verification.
type ChainHeaderReader interface {
	// Config retrieves the blockchain's chain configuration.
	Config() *params2.ChainConfig

	// CurrentHeader retrieves the current header from the local chain.
	CurrentHeader() *types.Header

	// GetHeader retrieves a block header from the database by hash and number.
	GetHeader(hash common.Hash, number uint64) *types.Header

	// GetHeaderByNumber retrieves a block header from the database by number.
	GetHeaderByNumber(number uint64) *types.Header

	// GetHeaderByHash retrieves a block header from the database by its hash.
	GetHeaderByHash(hash common.Hash) *types.Header
}

// ChainReader defines a small collection of methods needed to access the local
// blockchain during header and/or uncle verification.
type ChainReader interface {
	ChainHeaderReader

	// GetBlock retrieves a block from the database by hash and number.
	GetBlock(hash common.Hash, number uint64) *types.Block
}

// Engine is an algorithm agnostic consensus engine.
type Engine interface {
	// Author retrieves the Ethereum address of the account that minted the given
	// block, which may be different from the header's coinbase if a consensus
	// engine is based on signatures.
	Author(header *types.Header) (common.Address, error)

	// VerifyHeader checks whether a header conforms to the consensus rules of a
	// given engine. Verifying the seal may be done optionally here, or explicitly
	// via the VerifySeal method.
	VerifyHeader(chain ChainHeaderReader, header *types.Header, seal bool) error

	// VerifyHeaders is similar to VerifyHeader, but verifies a batch of headers
	// concurrently. The method returns a quit channel to abort the operations and
	// a results channel to retrieve the async verifications (the order is that of
	// the input slice).
	VerifyHeaders(chain ChainHeaderReader, headers []*types.Header, seals []bool) (chan<- struct{}, <-chan error)

	// VerifyUncles verifies that the given block's uncles conform to the consensus
	// rules of a given engine.
	//VerifyUncles(chain ChainReader, block *types.Block) error

	// Prepare initializes the consensus fields of a block header according to the
	// rules of a particular engine. The changes are executed inline.
	Prepare(chain ChainHeaderReader, header *types.Header) error

	// Finalize runs any post-transaction state modifications (e.g. block rewards)
	// but does not assemble the block.
	//
	// Note: The block header and state database might be updated to reflect any
	// consensus rules that happen at finalization (e.g. block rewards).
	Finalize(chain ChainHeaderReader, header *types.Header, state *state.StateDB, txs []*types.Transaction)

	// FinalizeAndAssemble runs any post-transaction state modifications (e.g. block
	// rewards) and assembles the final block.
	//
	// Note: The block header and state database might be updated to reflect any
	// consensus rules that happen at finalization (e.g. block rewards).
	FinalizeAndAssemble(chain ChainHeaderReader, header *types.Header, state *state.StateDB, txs []*types.Transaction,
		receipts []*types.Receipt, randomness *types.Randomness) (*types.Block, error)

	// Seal generates a new sealing request for the given input block and pushes
	// the result into the given channel.
	//
	// Note, the method returns immediately and will send the result async. More
	// than one result may also be returned depending on the consensus algorithm.
	Seal(chain ChainHeaderReader, block *types.Block) error

	// SealHash returns the hash of a block prior to it being sealed.
	SealHash(header *types.Header) common.Hash
	// VerifySeal checks whether the crypto seal on a header is valid according to
	// the consensus rules of the given engine.
	VerifySeal(header *types.Header) error

	// CalcDifficulty is the difficulty adjustment algorithm. It returns the difficulty
	// that a new block should have.
	CalcDifficulty(chain ChainHeaderReader, time uint64, parent *types.Header) *big.Int

	// GetValidators returns the list of current validators.
	GetValidators(blockNumber *big.Int, headerHash common.Hash) []istanbul.Validator

	EpochSize() uint64

	// APIs returns the RPC APIs this consensus engine provides.
	APIs(chain ChainHeaderReader) []rpc.API

	// Close terminates any background threads maintained by the consensus engine.
	Close() error
}

// PoW is a consensus engine based on proof-of-work.
type PoW interface {
	Engine

	// Hashrate returns the current mining hashrate of a PoW consensus engine.
	Hashrate() float64
}

// Istanbul is a consensus engine to avoid byzantine failure
type Istanbul interface {
	Engine

	// IsProxiedValidator returns true if this node is a proxied validator
	IsProxiedValidator() bool

	// IsProxy returns true if this node is a proxy
	IsProxy() bool

	// IsPrimary returns true if this node is the primary validator
	IsPrimary() bool

	// IsPrimaryForSeq returns true if this node is the primary validator for the sequence
	IsPrimaryForSeq(seq *big.Int) bool

	// SetChain injects the blockchain and related functions to the istanbul consensus engine
	SetChain(chain ChainContext, currentBlock func() *types.Block, stateAt func(common.Hash) (*state.StateDB, error))

	// SetCallBacks sets call back functions
	SetCallBacks(hasBadBlock func(common.Hash) bool,
		processBlock func(*types.Block, *state.StateDB) (types.Receipts, []*types.Log, uint64, error),
		validateState func(*types.Block, *state.StateDB, types.Receipts, uint64) error,
		onNewConsensusBlock func(block *types.Block, receipts []*types.Receipt, logs []*types.Log, state *state.StateDB)) error

	// StartValidating starts the validating engine
	StartValidating() error

	// StopValidating stops the validating engine
	StopValidating() error

	// StartAnnouncing starts the announcing
	StartAnnouncing() error

	// StopAnnouncing stops the announcing
	StopAnnouncing() error

	// StartProxiedValidatorEngine starts the proxied validator engine
	StartProxiedValidatorEngine() error

	// StopProxiedValidatorEngine stops the proxied validator engine
	StopProxiedValidatorEngine() error

	// UpdateValSetDiff will update the validator set diff in the header, if the mined header is the last block of the epoch.
	// The changes are executed inline.
	UpdateValSetDiff(chain ChainHeaderReader, header *types.Header, state *state.StateDB) error

	// IsLastBlockOfEpoch will check to see if the header is from the last block of an epoch
	IsLastBlockOfEpoch(header *types.Header) bool

	// LookbackWindow returns the size of the lookback window for calculating uptime (in blocks)
	LookbackWindow(header *types.Header, state *state.StateDB) uint64

	// ValidatorAddress will return the istanbul engine's validator address
	ValidatorAddress() common.Address

	// GenerateRandomness will generate the random beacon randomness
	GenerateRandomness(parentHash common.Hash) (common.Hash, common.Hash, error)
}

// ChainContext defines a small collection of methods needed to access the local
// blockchain
type ChainContext interface {
	ChainHeaderReader

	// NewEVMRunnerForCurrentBlock creates the System's EVMRunner for current block & state
	NewEVMRunnerForCurrentBlock() (vm.EVMRunner, error)

	// NewEVMRunner creates the System's EVMRunner for given header & sttate
	NewEVMRunner(header *types.Header, state types.StateDB) vm.EVMRunner
}

// Handler should be implemented if the consensus needs to handle and send peer messages
type Handler interface {
	// NewWork handles a new work event from the miner
	NewWork() error

	// HandleMsg handles a message from peer
	HandleMsg(address common.Address, data p2p.Msg, peer Peer) (bool, error)

	// SetBroadcaster sets the broadcaster to send message to peers
	SetBroadcaster(Broadcaster)

	// SetP2PServer sets the p2p server to connect/disconnect to/from peers
	SetP2PServer(P2PServer)

	// RegisterPeer will notify the consensus engine that a new peer has been added
	RegisterPeer(peer Peer, fromProxiedNode bool) error

	// UnregisterPeer will notify the consensus engine that a new peer has been removed
	UnregisterPeer(peer Peer, fromProxiedNode bool)

	// Handshake will begin a handshake with a new peer. It returns if the peer
	// has identified itself as a validator and should bypass any max peer checks.
	Handshake(peer Peer) (bool, error)
}

func InitHeaderStore(state *state.StateDB, blockNumber *big.Int) {
	if blockNumber.Cmp(big.NewInt(0)) == 0 {
		initEthereumStore(state)
	}
}

func InitTxVerify(state *state.StateDB, blockNumber *big.Int) {
	if blockNumber.Cmp(big.NewInt(0)) == 0 {
		key := common.BytesToHash(params.TxVerifyAddress[:])
		getState := state.GetPOWState(params.TxVerifyAddress, key)
		if len(getState) == 0 {
			state.SetCode(params.TxVerifyAddress, params.TxVerifyAddress[:])
		}
	}
}

func initEthereumStore(state *state.StateDB) {
	key := common.BytesToHash(chains.EthereumHeaderStoreAddress[:])
	getState := state.GetPOWState(chains.EthereumHeaderStoreAddress, key)
	if len(getState) == 0 {
		var header ethereum.Header

		if err := json.Unmarshal([]byte(params.EthereumTestnetGenesisHeader), &header); err != nil {
			log.Crit("json unmarshal ethereum testnet header failed", "error", err)
		}

		if err := ethereum.InitHeaderStore(state, &header, params.EthereumTestnetGenesisTD); err != nil {
			log.Crit("init header store failed, ", "error", err)
		}
		state.SetCode(params.HeaderStoreAddress, params.HeaderStoreAddress[:])
	}
}
