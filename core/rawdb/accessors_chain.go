// Copyright 2018 The go-ethereum Authors
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

package rawdb

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"

	"github.com/mapprotocol/atlas/consensus/istanbul"
	"github.com/mapprotocol/atlas/consensus/istanbul/uptime"
	"github.com/mapprotocol/atlas/core/types"
	"github.com/mapprotocol/atlas/params"
)

// ReadCanonicalHash retrieves the hash assigned to a canonical block number.
func ReadCanonicalHash(db ethdb.Reader, number uint64) common.Hash {
	data, _ := db.Ancient(freezerHashTable, number)
	if len(data) == 0 {
		data, _ = db.Get(headerHashKey(number))
		// In the background freezer is moving data from leveldb to flatten files.
		// So during the first check for ancient db, the data is not yet in there,
		// but when we reach into leveldb, the data was already moved. That would
		// result in a not found error.
		if len(data) == 0 {
			data, _ = db.Ancient(freezerHashTable, number)
		}
	}
	if len(data) == 0 {
		return common.Hash{}
	}
	return common.BytesToHash(data)
}

// WriteCanonicalHash stores the hash assigned to a canonical block number.
func WriteCanonicalHash(db ethdb.KeyValueWriter, hash common.Hash, number uint64) {
	if err := db.Put(headerHashKey(number), hash.Bytes()); err != nil {
		log.Crit("Failed to store number to hash mapping", "err", err)
	}
}

// DeleteCanonicalHash removes the number to hash canonical mapping.
func DeleteCanonicalHash(db ethdb.KeyValueWriter, number uint64) {
	if err := db.Delete(headerHashKey(number)); err != nil {
		log.Crit("Failed to delete number to hash mapping", "err", err)
	}
}

// ReadAllHashes retrieves all the hashes assigned to blocks at a certain heights,
// both canonical and reorged forks included.
func ReadAllHashes(db ethdb.Iteratee, number uint64) []common.Hash {
	prefix := headerKeyPrefix(number)

	hashes := make([]common.Hash, 0, 1)
	it := db.NewIterator(prefix, nil)
	defer it.Release()

	for it.Next() {
		if key := it.Key(); len(key) == len(prefix)+32 {
			hashes = append(hashes, common.BytesToHash(key[len(key)-32:]))
		}
	}
	return hashes
}

type NumberHash struct {
	Number uint64
	Hash   common.Hash
}

// ReadAllHashes retrieves all the hashes assigned to blocks at a certain heights,
// both canonical and reorged forks included.
// This method considers both limits to be _inclusive_.
func ReadAllHashesInRange(db ethdb.Iteratee, first, last uint64) []*NumberHash {
	var (
		start     = encodeBlockNumber(first)
		keyLength = len(headerPrefix) + 8 + 32
		hashes    = make([]*NumberHash, 0, 1+last-first)
		it        = db.NewIterator(headerPrefix, start)
	)
	defer it.Release()
	for it.Next() {
		key := it.Key()
		if len(key) != keyLength {
			continue
		}
		num := binary.BigEndian.Uint64(key[len(headerPrefix) : len(headerPrefix)+8])
		if num > last {
			break
		}
		hash := common.BytesToHash(key[len(key)-32:])
		hashes = append(hashes, &NumberHash{num, hash})
	}
	return hashes
}

// ReadAllCanonicalHashes retrieves all canonical number and hash mappings at the
// certain chain range. If the accumulated entries reaches the given threshold,
// abort the iteration and return the semi-finish result.
func ReadAllCanonicalHashes(db ethdb.Iteratee, from uint64, to uint64, limit int) ([]uint64, []common.Hash) {
	// Short circuit if the limit is 0.
	if limit == 0 {
		return nil, nil
	}
	var (
		numbers []uint64
		hashes  []common.Hash
	)
	// Construct the key prefix of start point.
	start, end := headerHashKey(from), headerHashKey(to)
	it := db.NewIterator(nil, start)
	defer it.Release()

	for it.Next() {
		if bytes.Compare(it.Key(), end) >= 0 {
			break
		}
		if key := it.Key(); len(key) == len(headerPrefix)+8+1 && bytes.Equal(key[len(key)-1:], headerHashSuffix) {
			numbers = append(numbers, binary.BigEndian.Uint64(key[len(headerPrefix):len(headerPrefix)+8]))
			hashes = append(hashes, common.BytesToHash(it.Value()))
			// If the accumulated entries reaches the limit threshold, return.
			if len(numbers) >= limit {
				break
			}
		}
	}
	return numbers, hashes
}

// ReadHeaderNumber returns the header number assigned to a hash.
func ReadHeaderNumber(db ethdb.KeyValueReader, hash common.Hash) *uint64 {
	data, _ := db.Get(headerNumberKey(hash))
	if len(data) != 8 {
		return nil
	}
	number := binary.BigEndian.Uint64(data)
	return &number
}

// WriteHeaderNumber stores the hash->number mapping.
func WriteHeaderNumber(db ethdb.KeyValueWriter, hash common.Hash, number uint64) {
	key := headerNumberKey(hash)
	enc := encodeBlockNumber(number)
	if err := db.Put(key, enc); err != nil {
		log.Crit("Failed to store hash to number mapping", "err", err)
	}
}

// DeleteHeaderNumber removes hash->number mapping.
func DeleteHeaderNumber(db ethdb.KeyValueWriter, hash common.Hash) {
	if err := db.Delete(headerNumberKey(hash)); err != nil {
		log.Crit("Failed to delete hash to number mapping", "err", err)
	}
}

// ReadHeadHeaderHash retrieves the hash of the current canonical head header.
func ReadHeadHeaderHash(db ethdb.KeyValueReader) common.Hash {
	data, _ := db.Get(headHeaderKey)
	if len(data) == 0 {
		return common.Hash{}
	}
	return common.BytesToHash(data)
}

// WriteHeadHeaderHash stores the hash of the current canonical head header.
func WriteHeadHeaderHash(db ethdb.KeyValueWriter, hash common.Hash) {
	if err := db.Put(headHeaderKey, hash.Bytes()); err != nil {
		log.Crit("Failed to store last header's hash", "err", err)
	}
}

// ReadHeadBlockHash retrieves the hash of the current canonical head block.
func ReadHeadBlockHash(db ethdb.KeyValueReader) common.Hash {
	data, _ := db.Get(headBlockKey)
	if len(data) == 0 {
		return common.Hash{}
	}
	return common.BytesToHash(data)
}

// WriteHeadBlockHash stores the head block's hash.
func WriteHeadBlockHash(db ethdb.KeyValueWriter, hash common.Hash) {
	if err := db.Put(headBlockKey, hash.Bytes()); err != nil {
		log.Crit("Failed to store last block's hash", "err", err)
	}
}

// ReadHeadFastBlockHash retrieves the hash of the current fast-sync head block.
func ReadHeadFastBlockHash(db ethdb.KeyValueReader) common.Hash {
	data, _ := db.Get(headFastBlockKey)
	if len(data) == 0 {
		return common.Hash{}
	}
	return common.BytesToHash(data)
}

// WriteHeadFastBlockHash stores the hash of the current fast-sync head block.
func WriteHeadFastBlockHash(db ethdb.KeyValueWriter, hash common.Hash) {
	if err := db.Put(headFastBlockKey, hash.Bytes()); err != nil {
		log.Crit("Failed to store last fast block's hash", "err", err)
	}
}

// ReadLastPivotNumber retrieves the number of the last pivot block. If the node
// full synced, the last pivot will always be nil.
func ReadLastPivotNumber(db ethdb.KeyValueReader) *uint64 {
	data, _ := db.Get(lastPivotKey)
	if len(data) == 0 {
		return nil
	}
	var pivot uint64
	if err := rlp.DecodeBytes(data, &pivot); err != nil {
		log.Error("Invalid pivot block number in database", "err", err)
		return nil
	}
	return &pivot
}

// WriteLastPivotNumber stores the number of the last pivot block.
func WriteLastPivotNumber(db ethdb.KeyValueWriter, pivot uint64) {
	enc, err := rlp.EncodeToBytes(pivot)
	if err != nil {
		log.Crit("Failed to encode pivot block number", "err", err)
	}
	if err := db.Put(lastPivotKey, enc); err != nil {
		log.Crit("Failed to store pivot block number", "err", err)
	}
}

// ReadFastTrieProgress retrieves the number of tries nodes fast synced to allow
// reporting correct numbers across restarts.
func ReadFastTrieProgress(db ethdb.KeyValueReader) uint64 {
	data, _ := db.Get(fastTrieProgressKey)
	if len(data) == 0 {
		return 0
	}
	return new(big.Int).SetBytes(data).Uint64()
}

// WriteFastTrieProgress stores the fast sync trie process counter to support
// retrieving it across restarts.
func WriteFastTrieProgress(db ethdb.KeyValueWriter, count uint64) {
	if err := db.Put(fastTrieProgressKey, new(big.Int).SetUint64(count).Bytes()); err != nil {
		log.Crit("Failed to store fast sync trie progress", "err", err)
	}
}

// ReadTxIndexTail retrieves the number of oldest indexed block
// whose transaction indices has been indexed. If the corresponding entry
// is non-existent in database it means the indexing has been finished.
func ReadTxIndexTail(db ethdb.KeyValueReader) *uint64 {
	data, _ := db.Get(txIndexTailKey)
	if len(data) != 8 {
		return nil
	}
	number := binary.BigEndian.Uint64(data)
	return &number
}

// WriteTxIndexTail stores the number of oldest indexed block
// into database.
func WriteTxIndexTail(db ethdb.KeyValueWriter, number uint64) {
	if err := db.Put(txIndexTailKey, encodeBlockNumber(number)); err != nil {
		log.Crit("Failed to store the transaction index tail", "err", err)
	}
}

// ReadFastTxLookupLimit retrieves the tx lookup limit used in fast sync.
func ReadFastTxLookupLimit(db ethdb.KeyValueReader) *uint64 {
	data, _ := db.Get(fastTxLookupLimitKey)
	if len(data) != 8 {
		return nil
	}
	number := binary.BigEndian.Uint64(data)
	return &number
}

// WriteFastTxLookupLimit stores the txlookup limit used in fast sync into database.
func WriteFastTxLookupLimit(db ethdb.KeyValueWriter, number uint64) {
	if err := db.Put(fastTxLookupLimitKey, encodeBlockNumber(number)); err != nil {
		log.Crit("Failed to store transaction lookup limit for fast sync", "err", err)
	}
}

// ReadHeaderRLP retrieves a block header in its raw RLP database encoding.
func ReadHeaderRLP(db ethdb.Reader, hash common.Hash, number uint64) rlp.RawValue {
	// First try to look up the data in ancient database. Extra hash
	// comparison is necessary since ancient database only maintains
	// the canonical data.
	data, _ := db.Ancient(freezerHeaderTable, number)
	if len(data) > 0 {
		return data
	}
	// Then try to look up the data in leveldb.
	data, _ = db.Get(headerKey(number, hash))
	if len(data) > 0 {
		return data
	}
	// In the background freezer is moving data from leveldb to flatten files.
	// So during the first check for ancient db, the data is not yet in there,
	// but when we reach into leveldb, the data was already moved. That would
	// result in a not found error.
	data, _ = db.Ancient(freezerHeaderTable, number)
	if len(data) > 0 {
		return data
	}
	return nil // Can't find the data anywhere.
}

// HasHeader verifies the existence of a block header corresponding to the hash.
func HasHeader(db ethdb.Reader, hash common.Hash, number uint64) bool {
	if has, err := db.Ancient(freezerHashTable, number); err == nil && common.BytesToHash(has) == hash {
		return true
	}
	if has, err := db.Has(headerKey(number, hash)); !has || err != nil {
		return false
	}
	return true
}

// ReadHeader retrieves the block header corresponding to the hash.
func ReadHeader(db ethdb.Reader, hash common.Hash, number uint64) *types.Header {
	data := ReadHeaderRLP(db, hash, number)
	if len(data) == 0 {
		return nil
	}
	header := new(types.Header)
	if err := rlp.Decode(bytes.NewReader(data), header); err != nil {
		log.Error("Invalid block header RLP", "hash", hash, "err", err)
		return nil
	}
	return header
}

// WriteHeader stores a block header into the database and also stores the hash-
// to-number mapping.
func WriteHeader(db ethdb.KeyValueWriter, header *types.Header) {
	var (
		hash   = header.Hash()
		number = header.Number.Uint64()
	)
	// Write the hash -> number mapping
	WriteHeaderNumber(db, hash, number)

	// Write the encoded header
	data, err := rlp.EncodeToBytes(header)
	if err != nil {
		log.Crit("Failed to RLP encode header", "err", err)
	}
	key := headerKey(number, hash)
	if err := db.Put(key, data); err != nil {
		log.Crit("Failed to store header", "err", err)
	}
}

// DeleteHeader removes all block header data associated with a hash.
func DeleteHeader(db ethdb.KeyValueWriter, hash common.Hash, number uint64) {
	deleteHeaderWithoutNumber(db, hash, number)
	if err := db.Delete(headerNumberKey(hash)); err != nil {
		log.Crit("Failed to delete hash to number mapping", "err", err)
	}
}

// deleteHeaderWithoutNumber removes only the block header but does not remove
// the hash to number mapping.
func deleteHeaderWithoutNumber(db ethdb.KeyValueWriter, hash common.Hash, number uint64) {
	if err := db.Delete(headerKey(number, hash)); err != nil {
		log.Crit("Failed to delete header", "err", err)
	}
}

// ReadBodyRLP retrieves the block body (transactions and uncles) in RLP encoding.
func ReadBodyRLP(db ethdb.Reader, hash common.Hash, number uint64) rlp.RawValue {
	// First try to look up the data in ancient database. Extra hash
	// comparison is necessary since ancient database only maintains
	// the canonical data.
	data, _ := db.Ancient(freezerBodiesTable, number)
	if len(data) > 0 {
		h, _ := db.Ancient(freezerHashTable, number)
		if common.BytesToHash(h) == hash {
			return data
		}
	}
	// Then try to look up the data in leveldb.
	data, _ = db.Get(blockBodyKey(number, hash))
	if len(data) > 0 {
		return data
	}
	// In the background freezer is moving data from leveldb to flatten files.
	// So during the first check for ancient db, the data is not yet in there,
	// but when we reach into leveldb, the data was already moved. That would
	// result in a not found error.
	data, _ = db.Ancient(freezerBodiesTable, number)
	if len(data) > 0 {
		h, _ := db.Ancient(freezerHashTable, number)
		if common.BytesToHash(h) == hash {
			return data
		}
	}
	return nil // Can't find the data anywhere.
}

// ReadCanonicalBodyRLP retrieves the block body (transactions and uncles) for the canonical
// block at number, in RLP encoding.
func ReadCanonicalBodyRLP(db ethdb.Reader, number uint64) rlp.RawValue {
	// If it's an ancient one, we don't need the canonical hash
	data, _ := db.Ancient(freezerBodiesTable, number)
	if len(data) == 0 {
		// Need to get the hash
		data, _ = db.Get(blockBodyKey(number, ReadCanonicalHash(db, number)))
		// In the background freezer is moving data from leveldb to flatten files.
		// So during the first check for ancient db, the data is not yet in there,
		// but when we reach into leveldb, the data was already moved. That would
		// result in a not found error.
		if len(data) == 0 {
			data, _ = db.Ancient(freezerBodiesTable, number)
		}
	}
	return data
}

// WriteBodyRLP stores an RLP encoded block body into the database.
func WriteBodyRLP(db ethdb.KeyValueWriter, hash common.Hash, number uint64, rlp rlp.RawValue) {
	if err := db.Put(blockBodyKey(number, hash), rlp); err != nil {
		log.Crit("Failed to store block body", "err", err)
	}
}

// HasBody verifies the existence of a block body corresponding to the hash.
func HasBody(db ethdb.Reader, hash common.Hash, number uint64) bool {
	if has, err := db.Ancient(freezerHashTable, number); err == nil && common.BytesToHash(has) == hash {
		return true
	}
	if has, err := db.Has(blockBodyKey(number, hash)); !has || err != nil {
		return false
	}
	return true
}

// ReadBody retrieves the block body corresponding to the hash.
func ReadBody(db ethdb.Reader, hash common.Hash, number uint64) *types.Body {
	data := ReadBodyRLP(db, hash, number)
	if len(data) == 0 {
		return nil
	}
	body := new(types.Body)
	if err := rlp.Decode(bytes.NewReader(data), body); err != nil {
		log.Error("Invalid block body RLP", "hash", hash, "err", err)
		return nil
	}
	return body
}

// WriteBody stores a block body into the database.
func WriteBody(db ethdb.KeyValueWriter, hash common.Hash, number uint64, body *types.Body) {
	data, err := rlp.EncodeToBytes(body)
	if err != nil {
		log.Crit("Failed to RLP encode body", "err", err)
	}
	WriteBodyRLP(db, hash, number, data)
}

// DeleteBody removes all block body data associated with a hash.
func DeleteBody(db ethdb.KeyValueWriter, hash common.Hash, number uint64) {
	if err := db.Delete(blockBodyKey(number, hash)); err != nil {
		log.Crit("Failed to delete block body", "err", err)
	}
}

// ReadTdRLP retrieves a block's total difficulty corresponding to the hash in RLP encoding.
func ReadTdRLP(db ethdb.Reader, hash common.Hash, number uint64) rlp.RawValue {
	// First try to look up the data in ancient database. Extra hash
	// comparison is necessary since ancient database only maintains
	// the canonical data.
	data, _ := db.Ancient(freezerDifficultyTable, number)
	if len(data) > 0 {
		h, _ := db.Ancient(freezerHashTable, number)
		if common.BytesToHash(h) == hash {
			return data
		}
	}
	// Then try to look up the data in leveldb.
	data, _ = db.Get(headerTDKey(number, hash))
	if len(data) > 0 {
		return data
	}
	// In the background freezer is moving data from leveldb to flatten files.
	// So during the first check for ancient db, the data is not yet in there,
	// but when we reach into leveldb, the data was already moved. That would
	// result in a not found error.
	data, _ = db.Ancient(freezerDifficultyTable, number)
	if len(data) > 0 {
		h, _ := db.Ancient(freezerHashTable, number)
		if common.BytesToHash(h) == hash {
			return data
		}
	}
	return nil // Can't find the data anywhere.
}

// ReadTd retrieves a block's total difficulty corresponding to the hash.
func ReadTd(db ethdb.Reader, hash common.Hash, number uint64) *big.Int {
	data := ReadTdRLP(db, hash, number)
	if len(data) == 0 {
		return nil
	}
	td := new(big.Int)
	if err := rlp.Decode(bytes.NewReader(data), td); err != nil {
		log.Error("Invalid block total difficulty RLP", "hash", hash, "err", err)
		return nil
	}
	return td
}

// WriteTd stores the total difficulty of a block into the database.
func WriteTd(db ethdb.KeyValueWriter, hash common.Hash, number uint64, td *big.Int) {
	data, err := rlp.EncodeToBytes(td)
	if err != nil {
		log.Crit("Failed to RLP encode block total difficulty", "err", err)
	}
	if err := db.Put(headerTDKey(number, hash), data); err != nil {
		log.Crit("Failed to store block total difficulty", "err", err)
	}
}

// DeleteTd removes all block total difficulty data associated with a hash.
func DeleteTd(db ethdb.KeyValueWriter, hash common.Hash, number uint64) {
	if err := db.Delete(headerTDKey(number, hash)); err != nil {
		log.Crit("Failed to delete block total difficulty", "err", err)
	}
}

// HasReceipts verifies the existence of all the transaction receipts belonging
// to a block.
func HasReceipts(db ethdb.Reader, hash common.Hash, number uint64) bool {
	if has, err := db.Ancient(freezerHashTable, number); err == nil && common.BytesToHash(has) == hash {
		return true
	}
	if has, err := db.Has(blockReceiptsKey(number, hash)); !has || err != nil {
		return false
	}
	return true
}

// ReadReceiptsRLP retrieves all the transaction receipts belonging to a block in RLP encoding.
func ReadReceiptsRLP(db ethdb.Reader, hash common.Hash, number uint64) rlp.RawValue {
	// First try to look up the data in ancient database. Extra hash
	// comparison is necessary since ancient database only maintains
	// the canonical data.
	data, _ := db.Ancient(freezerReceiptTable, number)
	if len(data) > 0 {
		h, _ := db.Ancient(freezerHashTable, number)
		if common.BytesToHash(h) == hash {
			return data
		}
	}
	// Then try to look up the data in leveldb.
	data, _ = db.Get(blockReceiptsKey(number, hash))
	if len(data) > 0 {
		return data
	}
	// In the background freezer is moving data from leveldb to flatten files.
	// So during the first check for ancient db, the data is not yet in there,
	// but when we reach into leveldb, the data was already moved. That would
	// result in a not found error.
	data, _ = db.Ancient(freezerReceiptTable, number)
	if len(data) > 0 {
		h, _ := db.Ancient(freezerHashTable, number)
		if common.BytesToHash(h) == hash {
			return data
		}
	}
	return nil // Can't find the data anywhere.
}

// ReadRawReceipts retrieves all the transaction receipts belonging to a block.
// The receipt metadata fields are not guaranteed to be populated, so they
// should not be used. Use ReadReceipts instead if the metadata is needed.
func ReadRawReceipts(db ethdb.Reader, hash common.Hash, number uint64) types.Receipts {
	// Retrieve the flattened receipt slice
	data := ReadReceiptsRLP(db, hash, number)
	if len(data) == 0 {
		return nil
	}
	// Convert the receipts from their storage form to their internal representation
	storageReceipts := []*types.ReceiptForStorage{}
	if err := rlp.DecodeBytes(data, &storageReceipts); err != nil {
		log.Error("Invalid receipt array RLP", "hash", hash, "err", err)
		return nil
	}
	receipts := make(types.Receipts, len(storageReceipts))
	for i, storageReceipt := range storageReceipts {
		receipts[i] = (*types.Receipt)(storageReceipt)
	}
	return receipts
}

// ReadReceipts retrieves all the transaction receipts belonging to a block, including
// its correspoinding metadata fields. If it is unable to populate these metadata
// fields then nil is returned.
//
// The current implementation populates these metadata fields by reading the receipts'
// corresponding block body, so if the block body is not found it will return nil even
// if the receipt itself is stored.
func ReadReceipts(db ethdb.Reader, hash common.Hash, number uint64, config *params.ChainConfig) types.Receipts {
	// We're deriving many fields from the block body, retrieve beside the receipt
	receipts := ReadRawReceipts(db, hash, number)
	if receipts == nil {
		return nil
	}
	body := ReadBody(db, hash, number)
	if body == nil {
		log.Error("Missing body but have receipt", "hash", hash, "number", number)
		return nil
	}
	if err := receipts.DeriveFields(config, hash, number, body.Transactions); err != nil {
		log.Error("Failed to derive block receipts fields", "hash", hash, "number", number, "err", err)
		return nil
	}
	return receipts
}

// WriteReceipts stores all the transaction receipts belonging to a block.
func WriteReceipts(db ethdb.KeyValueWriter, hash common.Hash, number uint64, receipts types.Receipts) {
	// Convert the receipts into their storage form and serialize them
	storageReceipts := make([]*types.ReceiptForStorage, len(receipts))
	for i, receipt := range receipts {
		storageReceipts[i] = (*types.ReceiptForStorage)(receipt)
	}
	bytes, err := rlp.EncodeToBytes(storageReceipts)
	if err != nil {
		log.Crit("Failed to encode block receipts", "err", err)
	}
	// Store the flattened receipt slice
	if err := db.Put(blockReceiptsKey(number, hash), bytes); err != nil {
		log.Crit("Failed to store block receipts", "err", err)
	}
}

// DeleteReceipts removes all receipt data associated with a block hash.
func DeleteReceipts(db ethdb.KeyValueWriter, hash common.Hash, number uint64) {
	if err := db.Delete(blockReceiptsKey(number, hash)); err != nil {
		log.Crit("Failed to delete block receipts", "err", err)
	}
}

// storedReceiptRLP is the storage encoding of a receipt.
// Re-definition in core/types/receipt.go.
type storedReceiptRLP struct {
	PostStateOrStatus []byte
	CumulativeGasUsed uint64
	Logs              []*types.LogForStorage
}

// ReceiptLogs is a barebone version of ReceiptForStorage which only keeps
// the list of logs. When decoding a stored receipt into this object we
// avoid creating the bloom filter.
type receiptLogs struct {
	Logs []*types.Log
}

// DecodeRLP implements rlp.Decoder.
func (r *receiptLogs) DecodeRLP(s *rlp.Stream) error {
	var stored storedReceiptRLP
	if err := s.Decode(&stored); err != nil {
		return err
	}
	r.Logs = make([]*types.Log, len(stored.Logs))
	for i, log := range stored.Logs {
		r.Logs[i] = (*types.Log)(log)
	}
	return nil
}

// DeriveLogFields fills the logs in receiptLogs with information such as block number, txhash, etc.
func deriveLogFields(receipts []*receiptLogs, hash common.Hash, number uint64, txs types.Transactions) error {
	logIndex := uint(0)
	// The receipts may include an additional "block finalization" receipt (only IBFT)
	if !(len(txs) == len(receipts) || len(txs)+1 == len(receipts)) {
		return errors.New("transaction and receipt count mismatch")
	}
	for i := 0; i < len(txs); i++ {
		txHash := txs[i].Hash()
		// The derived log fields can simply be set from the block and transaction
		for j := 0; j < len(receipts[i].Logs); j++ {
			receipts[i].Logs[j].BlockNumber = number
			receipts[i].Logs[j].BlockHash = hash
			receipts[i].Logs[j].TxHash = txHash
			receipts[i].Logs[j].TxIndex = uint(i)
			receipts[i].Logs[j].Index = logIndex
			logIndex++
		}
	}
	// Handle block finalization receipt (only IBFT)
	if len(txs)+1 == len(receipts) {
		j := len(txs)
		for k := 0; k < len(receipts[j].Logs); k++ {
			receipts[j].Logs[k].BlockNumber = number
			receipts[j].Logs[k].BlockHash = hash
			receipts[j].Logs[k].TxHash = hash
			receipts[j].Logs[k].TxIndex = uint(j)
			receipts[j].Logs[k].Index = logIndex
			logIndex++
		}
	}
	return nil
}

// ReadLogs retrieves the logs for all transactions in a block. The log fields
// are populated with metadata. In case the receipts or the block body
// are not found, a nil is returned.
func ReadLogs(db ethdb.Reader, hash common.Hash, number uint64) [][]*types.Log {
	// Retrieve the flattened receipt slice
	data := ReadReceiptsRLP(db, hash, number)
	if len(data) == 0 {
		return nil
	}
	receipts := []*receiptLogs{}
	if err := rlp.DecodeBytes(data, &receipts); err != nil {
		log.Error("Invalid receipt array RLP", "hash", hash, "err", err)
		return nil
	}

	body := ReadBody(db, hash, number)
	if body == nil {
		log.Error("Missing body but have receipt", "hash", hash, "number", number)
		return nil
	}
	if err := deriveLogFields(receipts, hash, number, body.Transactions); err != nil {
		log.Error("Failed to derive block receipts fields", "hash", hash, "number", number, "err", err)
		return nil
	}
	logs := make([][]*types.Log, len(receipts))
	for i, receipt := range receipts {
		logs[i] = receipt.Logs
	}
	return logs
}

// ReadBlock retrieves an entire block corresponding to the hash, assembling it
// back from the stored header and body. If either the header or body could not
// be retrieved nil is returned.
//
// Note, due to concurrent download of header and block body the header and thus
// canonical hash can be stored in the database but the body data not (yet).
func ReadBlock(db ethdb.Reader, hash common.Hash, number uint64) *types.Block {
	header := ReadHeader(db, hash, number)
	if header == nil {
		return nil
	}
	body := ReadBody(db, hash, number)
	if body == nil {
		return nil
	}
	return types.NewBlockWithHeader(header).WithBody(body.Transactions, body.Randomness, body.EpochSnarkData)
}

// WriteBlock serializes a block into the database, header and body separately.
func WriteBlock(db ethdb.KeyValueWriter, block *types.Block) {
	WriteBody(db, block.Hash(), block.NumberU64(), block.Body())
	WriteHeader(db, block.Header())
}

// WriteAncientBlocks writes entire block data into ancient store and returns the total written size.
func WriteAncientBlocks(db ethdb.AncientWriter, blocks []*types.Block, receipts []types.Receipts, td *big.Int) (int64, error) {
	var (
		tdSum      = new(big.Int).Set(td)
		stReceipts []*types.ReceiptForStorage
	)
	return db.ModifyAncients(func(op ethdb.AncientWriteOp) error {
		for i, block := range blocks {
			// Convert receipts to storage format and sum up total difficulty.
			stReceipts = stReceipts[:0]
			for _, receipt := range receipts[i] {
				stReceipts = append(stReceipts, (*types.ReceiptForStorage)(receipt))
			}
			header := block.Header()
			if i > 0 {
				tdSum.Add(tdSum, block.TotalDifficulty())
			}
			if err := writeAncientBlock(op, block, header, stReceipts, tdSum); err != nil {
				return err
			}
		}
		return nil
	})
}

func writeAncientBlock(op ethdb.AncientWriteOp, block *types.Block, header *types.Header, receipts []*types.ReceiptForStorage, td *big.Int) error {
	num := block.NumberU64()
	if err := op.AppendRaw(freezerHashTable, num, block.Hash().Bytes()); err != nil {
		return fmt.Errorf("can't add block %d hash: %v", num, err)
	}
	if err := op.Append(freezerHeaderTable, num, header); err != nil {
		return fmt.Errorf("can't append block header %d: %v", num, err)
	}
	if err := op.Append(freezerBodiesTable, num, block.Body()); err != nil {
		return fmt.Errorf("can't append block body %d: %v", num, err)
	}
	if err := op.Append(freezerReceiptTable, num, receipts); err != nil {
		return fmt.Errorf("can't append block %d receipts: %v", num, err)
	}
	if err := op.Append(freezerDifficultyTable, num, td); err != nil {
		return fmt.Errorf("can't append block %d total difficulty: %v", num, err)
	}
	return nil
}

// DeleteBlock removes all block data associated with a hash.
func DeleteBlock(db ethdb.KeyValueWriter, hash common.Hash, number uint64) {
	DeleteReceipts(db, hash, number)
	DeleteHeader(db, hash, number)
	DeleteBody(db, hash, number)
	DeleteTd(db, hash, number)
}

// DeleteBlockWithoutNumber removes all block data associated with a hash, except
// the hash to number mapping.
func DeleteBlockWithoutNumber(db ethdb.KeyValueWriter, hash common.Hash, number uint64) {
	DeleteReceipts(db, hash, number)
	deleteHeaderWithoutNumber(db, hash, number)
	DeleteBody(db, hash, number)
	DeleteTd(db, hash, number)
}

const badBlockToKeep = 10

type badBlock struct {
	Header *types.Header
	Body   *types.Body
}

// badBlockList implements the sort interface to allow sorting a list of
// bad blocks by their number in the reverse order.
type badBlockList []*badBlock

func (s badBlockList) Len() int { return len(s) }
func (s badBlockList) Less(i, j int) bool {
	return s[i].Header.Number.Uint64() < s[j].Header.Number.Uint64()
}
func (s badBlockList) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

// ReadBadBlock retrieves the bad block with the corresponding block hash.
func ReadBadBlock(db ethdb.Reader, hash common.Hash) *types.Block {
	blob, err := db.Get(badBlockKey)
	if err != nil {
		return nil
	}
	var badBlocks badBlockList
	if err := rlp.DecodeBytes(blob, &badBlocks); err != nil {
		return nil
	}
	for _, bad := range badBlocks {
		if bad.Header.Hash() == hash {
			return types.NewBlockWithHeader(bad.Header).WithBody(bad.Body.Transactions, bad.Body.Randomness, bad.Body.EpochSnarkData)
		}
	}
	return nil
}

// ReadAllBadBlocks retrieves all the bad blocks in the database.
// All returned blocks are sorted in reverse order by number.
func ReadAllBadBlocks(db ethdb.Reader) []*types.Block {
	blob, err := db.Get(badBlockKey)
	if err != nil {
		return nil
	}
	var badBlocks badBlockList
	if err := rlp.DecodeBytes(blob, &badBlocks); err != nil {
		return nil
	}
	var blocks []*types.Block
	for _, bad := range badBlocks {
		blocks = append(blocks, types.NewBlockWithHeader(bad.Header).WithBody(bad.Body.Transactions, bad.Body.Randomness, bad.Body.EpochSnarkData))
	}
	return blocks
}

// WriteBadBlock serializes the bad block into the database. If the cumulated
// bad blocks exceeds the limitation, the oldest will be dropped.
func WriteBadBlock(db ethdb.KeyValueStore, block *types.Block) {
	blob, err := db.Get(badBlockKey)
	if err != nil {
		log.Warn("Failed to load old bad blocks", "error", err)
	}
	var badBlocks badBlockList
	if len(blob) > 0 {
		if err := rlp.DecodeBytes(blob, &badBlocks); err != nil {
			log.Crit("Failed to decode old bad blocks", "error", err)
		}
	}
	for _, b := range badBlocks {
		if b.Header.Number.Uint64() == block.NumberU64() && b.Header.Hash() == block.Hash() {
			log.Info("Skip duplicated bad block", "number", block.NumberU64(), "hash", block.Hash())
			return
		}
	}
	badBlocks = append(badBlocks, &badBlock{
		Header: block.Header(),
		Body:   block.Body(),
	})
	sort.Sort(sort.Reverse(badBlocks))
	if len(badBlocks) > badBlockToKeep {
		badBlocks = badBlocks[:badBlockToKeep]
	}
	data, err := rlp.EncodeToBytes(badBlocks)
	if err != nil {
		log.Crit("Failed to encode bad blocks", "err", err)
	}
	if err := db.Put(badBlockKey, data); err != nil {
		log.Crit("Failed to write bad blocks", "err", err)
	}
}

// DeleteBadBlocks deletes all the bad blocks from the database
func DeleteBadBlocks(db ethdb.KeyValueWriter) {
	if err := db.Delete(badBlockKey); err != nil {
		log.Crit("Failed to delete bad blocks", "err", err)
	}
}

// FindCommonAncestor returns the last common ancestor of two block headers
func FindCommonAncestor(db ethdb.Reader, a, b *types.Header) *types.Header {
	for bn := b.Number.Uint64(); a.Number.Uint64() > bn; {
		a = ReadHeader(db, a.ParentHash, a.Number.Uint64()-1)
		if a == nil {
			return nil
		}
	}
	for an := a.Number.Uint64(); an < b.Number.Uint64(); {
		b = ReadHeader(db, b.ParentHash, b.Number.Uint64()-1)
		if b == nil {
			return nil
		}
	}
	for a.Hash() != b.Hash() {
		a = ReadHeader(db, a.ParentHash, a.Number.Uint64()-1)
		if a == nil {
			return nil
		}
		b = ReadHeader(db, b.ParentHash, b.Number.Uint64()-1)
		if b == nil {
			return nil
		}
	}
	return a
}

// ReadHeadHeader returns the current canonical head header.
func ReadHeadHeader(db ethdb.Reader) *types.Header {
	headHeaderHash := ReadHeadHeaderHash(db)
	if headHeaderHash == (common.Hash{}) {
		return nil
	}
	headHeaderNumber := ReadHeaderNumber(db, headHeaderHash)
	if headHeaderNumber == nil {
		return nil
	}
	return ReadHeader(db, headHeaderHash, *headHeaderNumber)
}

// ReadHeadBlock returns the current canonical head block.
func ReadHeadBlock(db ethdb.Reader) *types.Block {
	headBlockHash := ReadHeadBlockHash(db)
	if headBlockHash == (common.Hash{}) {
		return nil
	}
	headBlockNumber := ReadHeaderNumber(db, headBlockHash)
	if headBlockNumber == nil {
		return nil
	}
	return ReadBlock(db, headBlockHash, *headBlockNumber)
}

//---------------------------chains----------------------------------
// ReadCanonicalHashChains retrieves the hash assigned to a canonical block number.
//func ReadCanonicalHashChains(db DatabaseReader, number uint64, m ChainType) common.Hash {
//	data, _ := db.Get(headerHashKeyChains(m, number))
//	if len(data) == 0 {
//		return common.Hash{}
//	}
//	return common.BytesToHash(data)
//}
//
//// ReadFirstCanonicalHashChains
//func ReadFirstCanonicalHashChains(db DatabaseReader, m ChainType) common.Hash {
//	data, _ := db.Get(headerHashKeyChains(m, uint64(0)))
//	if len(data) == 0 {
//		return common.Hash{}
//	}
//	return common.BytesToHash(data)
//}
//
//// WriteCanonicalHashChains stores the hash assigned to a canonical block number.
//func WriteCanonicalHashChains(db DatabaseWriter, hash common.Hash, number uint64, m ChainType) {
//	if err := db.Put(headerHashKeyChains(m, number), hash.Bytes()); err != nil {
//		log.Crit("Failed to store number to hash mapping", "err", err)
//	}
//}
//
//// DeleteCanonicalHashChains removes the number to hash canonical mapping.
//func DeleteCanonicalHashChains(db DatabaseDeleter, number uint64, m ChainType) {
//	if err := db.Delete(headerHashKeyChains(m, number)); err != nil {
//		log.Crit("Failed to delete number to hash mapping", "err", err)
//	}
//}
//
//// ReadHeaderNumberChains returns the header number assigned to a hash.
//func ReadHeaderNumberChains(db DatabaseReader, hash common.Hash, m ChainType) *uint64 {
//	data, _ := db.Get(headerNumberKeyChains(m, hash))
//	if len(data) != 8 {
//		return nil
//	}
//	number := binary.BigEndian.Uint64(data)
//	return &number
//}
//
//// ReadHeadHeaderHashChains retrieves the hash of the current canonical head header.
//func ReadHeadHeaderHashChains(db DatabaseReader, m ChainType) common.Hash {
//	data, _ := db.Get(m.setTypeKey(headHeaderKey))
//	if len(data) == 0 {
//		return common.Hash{}
//	}
//	return common.BytesToHash(data)
//}
//
//// WriteHeadHeaderHashChains stores the hash of the current canonical head header.
//func WriteHeadHeaderHashChains(db DatabaseWriter, hash common.Hash, m ChainType) {
//	if err := db.Put(m.setTypeKey(headHeaderKey), hash.Bytes()); err != nil {
//		log.Crit("Failed to store last header's hash", "err", err)
//	}
//}
//
//// ReadHeadBlockHashChains retrieves the hash of the current canonical head block.
//func ReadHeadBlockHashChains(db DatabaseReader, m ChainType) common.Hash {
//	data, _ := db.Get(m.setTypeKey(headBlockKey))
//	if len(data) == 0 {
//		return common.Hash{}
//	}
//	return common.BytesToHash(data)
//}
//
//// WriteHeadBlockHashChains stores the head block's hash.
//func WriteHeadBlockHashChains(db DatabaseWriter, hash common.Hash, m ChainType) {
//	if err := db.Put(m.setTypeKey(headBlockKey), hash.Bytes()); err != nil {
//		log.Crit("Failed to store last block's hash", "err", err)
//	}
//}
//
//// ReadLastBlockHashChains retrieves the hash of the current canonical head block.
//func ReadLastBlockHashChains(db DatabaseReader, m ChainType) common.Hash {
//	data, _ := db.Get(m.setTypeKey(lastBlockKey))
//	if len(data) == 0 {
//		return common.Hash{}
//	}
//	return common.BytesToHash(data)
//}
//
//// WriteLastBlockHashChains stores the head block's hash.
//func WriteLastBlockHashChains(db DatabaseWriter, hash common.Hash, m ChainType) {
//	if err := db.Put(m.setTypeKey(lastBlockKey), hash.Bytes()); err != nil {
//		log.Crit("Failed to store last block's hash", "err", err)
//	}
//}
//
//// ReadHeadFastBlockHashChains retrieves the hash of the current fast-sync head block.
//func ReadHeadFastBlockHashChains(db DatabaseReader, m ChainType) common.Hash {
//	data, _ := db.Get(m.setTypeKey(headFastBlockKey))
//	if len(data) == 0 {
//		return common.Hash{}
//	}
//	return common.BytesToHash(data)
//}
//
//// WriteHeadFastBlockHashChains stores the hash of the current fast-sync head block.
//func WriteHeadFastBlockHashChains(db DatabaseWriter, hash common.Hash, m ChainType) {
//	if err := db.Put(m.setTypeKey(headFastBlockKey), hash.Bytes()); err != nil {
//		log.Crit("Failed to store last fast block's hash", "err", err)
//	}
//}
//
//// ReadFastTrieProgressChains retrieves the number of tries nodes fast synced to allow
//// reporting correct numbers across restarts.
//func ReadFastTrieProgressChains(db DatabaseReader, m ChainType) uint64 {
//	data, _ := db.Get(m.setTypeKey(fastTrieProgressKey))
//	if len(data) == 0 {
//		return 0
//	}
//	return new(big.Int).SetBytes(data).Uint64()
//}
//
//// WriteFastTrieProgressChains stores the fast sync trie process counter to support
//// retrieving it across restarts.
//func WriteFastTrieProgressChains(db DatabaseWriter, count uint64, m ChainType) {
//	if err := db.Put(m.setTypeKey(fastTrieProgressKey), new(big.Int).SetUint64(count).Bytes()); err != nil {
//		log.Crit("Failed to store fast sync trie progress", "err", err)
//	}
//}
//
//// ReadHeaderRLPChains retrieves a block header in its raw RLP database encoding.
//func ReadHeaderRLPChains(db DatabaseReader, hash common.Hash, number uint64, m ChainType) rlp.RawValue {
//	data, err := db.Get(headerKeyChains(m, number, hash))
//	if err != nil {
//		return rlp.RawValue{}
//	}
//	return data
//}
//
//// HasHeaderChains verifies the existence of a block header corresponding to the hash.
//func HasHeaderChains(db DatabaseReader, hash common.Hash, number uint64, m ChainType) bool {
//	if has, err := db.Has(headerKeyChains(m, number, hash)); !has || err != nil {
//		return false
//	}
//	return true
//}
//
//// ReadHeaderChains retrieves the block header corresponding to the hash.
//func ReadHeaderChains(db DatabaseReader, hash common.Hash, number uint64, m ChainType) *ethereum.Header {
//	data := ReadHeaderRLPChains(db, hash, number, m)
//	if len(data) == 0 {
//		return nil
//	}
//	header := new(ethereum.Header)
//	if err := rlp.Decode(bytes.NewReader(data), header); err != nil {
//		log.Error("Invalid block header RLP", "hash", hash, "err", err)
//		return nil
//	}
//	return header
//}
//
//// WriteHeaderChains stores a block header into the database and also stores the hash-
//// to-number mapping.
//func WriteHeaderChains(db DatabaseWriter, header *ethereum.Header, m ChainType) {
//	// Write the hash -> number mapping
//	var (
//		hash    = header.Hash()
//		number  = header.Number.Uint64()
//		encoded = encodeBlockNumber(number)
//	)
//	key := headerNumberKeyChains(m, hash)
//	if err := db.Put(key, encoded); err != nil {
//		log.Crit("Failed to store hash to number mapping", "err", err)
//	}
//	// Write the encoded header
//	data, err := rlp.EncodeToBytes(header)
//	//log.Info("=========   size of fast hash",len(data),"number of fastblock",number)
//	if err != nil {
//		log.Crit("Failed to RLP encode header", "err", err)
//	}
//	key = headerKeyChains(m, number, hash)
//	if err := db.Put(key, data); err != nil {
//		log.Crit("Failed to store header", "err", err)
//	}
//}
//
//// DeleteHeaderChains removes all block header data associated with a hash.
//func DeleteHeaderChains(db DatabaseDeleter, hash common.Hash, number uint64, m ChainType) {
//	if err := db.Delete(headerKeyChains(m, number, hash)); err != nil {
//		log.Crit("Failed to delete header", "err", err)
//	}
//	if err := db.Delete(headerNumberKeyChains(m, hash)); err != nil {
//		log.Crit("Failed to delete hash to number mapping", "err", err)
//	}
//}
//
//// ReadBodyRLPChains retrieves the block body (transactions and uncles) in RLP encoding.
//func ReadBodyRLPChains(db DatabaseReader, hash common.Hash, number uint64, m ChainType) rlp.RawValue {
//	data, _ := db.Get(blockBodyKeyChains(m, number, hash))
//	return data
//}
//
//// WriteBodyRLPChains stores an RLP encoded block body into the database.
//func WriteBodyRLPChains(db DatabaseWriter, hash common.Hash, number uint64, rlp rlp.RawValue, m ChainType) {
//	if err := db.Put(blockBodyKeyChains(m, number, hash), rlp); err != nil {
//		log.Crit("Failed to store block body", "err", err)
//	}
//}
//
//// HasBodyChains verifies the existence of a block body corresponding to the hash.
//func HasBodyChains(db DatabaseReader, hash common.Hash, number uint64, m ChainType) bool {
//	if has, err := db.Has(blockBodyKeyChains(m, number, hash)); !has || err != nil {
//		return false
//	}
//	return true
//}
//
//// ReadBodyChains retrieves the block body corresponding to the hash.
//func ReadBodyChains(db DatabaseReader, hash common.Hash, number uint64, m ChainType) *types.Body {
//	data := ReadBodyRLPChains(db, hash, number, m)
//	if len(data) == 0 {
//		return nil
//	}
//	body := new(types.Body)
//	if err := rlp.Decode(bytes.NewReader(data), body); err != nil {
//		log.Error("Invalid block body RLP", "hash", hash, "err", err)
//		return nil
//	}
//	return body
//}
//
//// WriteBodyChains storea a block body into the database.
//func WriteBodyChains(db DatabaseWriter, hash common.Hash, number uint64, body *types.Body, m ChainType) {
//	data, err := rlp.EncodeToBytes(body)
//	if err != nil {
//		log.Crit("Failed to RLP encode body", "err", err)
//	}
//	WriteBodyRLPChains(db, hash, number, data, m)
//}
//
//// DeleteBodyChains removes all block body data associated with a hash.
//func DeleteBodyChains(db DatabaseDeleter, hash common.Hash, number uint64, m ChainType) {
//	if err := db.Delete(blockBodyKeyChains(m, number, hash)); err != nil {
//		log.Crit("Failed to delete block body", "err", err)
//	}
//}
//
//// ReadTdChains retrieves a block's total difficulty corresponding to the hash.
//func ReadTdChains(db DatabaseReader, hash common.Hash, number uint64, m ChainType) *big.Int {
//	data, _ := db.Get(headerTDKeyChains(m, number, hash))
//	if len(data) == 0 {
//		return nil
//	}
//	td := new(big.Int)
//	if err := rlp.Decode(bytes.NewReader(data), td); err != nil {
//		log.Error("Invalid block total difficulty RLP", "hash", hash, "err", err)
//		return nil
//	}
//	return td
//}
//
//// WriteTdChains stores the total difficulty of a block into the database.
//func WriteTdChains(db DatabaseWriter, hash common.Hash, number uint64, td *big.Int, m ChainType) {
//	data, err := rlp.EncodeToBytes(td)
//	if err != nil {
//		log.Crit("Failed to RLP encode block total difficulty", "err", err)
//	}
//	if err := db.Put(headerTDKeyChains(m, number, hash), data); err != nil {
//		log.Crit("Failed to store block total difficulty", "err", err)
//	}
//}
//
//// HasReceiptsChains verifies the existence of all the transaction receipts belonging
//// to a block.
//func HasReceiptsChains(db DatabaseReader, hash common.Hash, number uint64, m ChainType) bool {
//	if has, err := db.Has(blockReceiptsKeyChains(m, number, hash)); !has || err != nil {
//		return false
//	}
//	return true
//}
//
//// ReadReceiptsChains retrieves all the transaction receipts belonging to a block.
//func ReadReceiptsChains(db DatabaseReader, hash common.Hash, number uint64, m ChainType) types.Receipts {
//	// Retrieve the flattened receipt slice
//	data, _ := db.Get(blockReceiptsKeyChains(m, number, hash))
//	if len(data) == 0 {
//		return nil
//	}
//	// Convert the revceipts from their storage form to their internal representation
//	storageReceipts := []*types.ReceiptForStorage{}
//	if err := rlp.DecodeBytes(data, &storageReceipts); err != nil {
//		log.Error("Invalid receipt array RLP", "hash", hash, "err", err)
//		return nil
//	}
//	receipts := make(types.Receipts, len(storageReceipts))
//	logIndex := uint(0)
//	for i, receipt := range storageReceipts {
//		// Assemble deriving fields for log.
//		for _, log := range receipt.Logs {
//			log.TxHash = receipt.TxHash
//			log.BlockHash = hash
//			log.BlockNumber = number
//			log.TxIndex = uint(i)
//			log.Index = logIndex
//			logIndex += 1
//		}
//		receipts[i] = (*types.Receipt)(receipt)
//		receipts[i].BlockHash = hash
//		receipts[i].BlockNumber = big.NewInt(0).SetUint64(number)
//		receipts[i].TransactionIndex = uint(i)
//	}
//	return receipts
//}
//
//// WriteReceiptsChains stores all the transaction receipts belonging to a block.
//func WriteReceiptsChains(db DatabaseWriter, hash common.Hash, number uint64, receipts types.Receipts, m ChainType) {
//	// Convert the receipts into their storage form and serialize them
//	storageReceipts := make([]*types.ReceiptForStorage, len(receipts))
//	for i, receipt := range receipts {
//		storageReceipts[i] = (*types.ReceiptForStorage)(receipt)
//	}
//	bytes, err := rlp.EncodeToBytes(storageReceipts)
//	if err != nil {
//		log.Crit("Failed to encode block receipts", "err", err)
//	}
//	if err := db.Put(blockReceiptsKeyChains(m, number, hash), bytes); err != nil {
//		log.Crit("Failed to store block receipts", "err", err)
//	}
//}
//
//// DeleteReceiptsChains removes all receipt data associated with a block hash.
//func DeleteReceiptsChains(db DatabaseDeleter, hash common.Hash, number uint64, m ChainType) {
//	if err := db.Delete(blockReceiptsKeyChains(m, number, hash)); err != nil {
//		log.Crit("Failed to delete block receipts", "err", err)
//	}
//}
//
//// WriteChainConfigChains writes the chain config settings to the database.
//func WriteChainConfigChains(db DatabaseWriter, hash common.Hash, cfg *params.ChainConfig, m ChainType) {
//	if cfg == nil {
//		return
//	}
//	data, err := json.Marshal(cfg)
//	if err != nil {
//		log.Crit("Failed to JSON encode chain config", "err", err)
//	}
//	if err := db.Put(configKeyChains(hash, m), data); err != nil {
//		log.Crit("Failed to store chain config", "err", err)
//	}
//}
//
//// configKey = configPrefix + hash
//func configKeyChains(hash common.Hash, m ChainType) []byte {
//	return append(m.setTypeKey(configPrefix), hash.Bytes()...)
//}

// WriteRandomCommitmentCache will write a random beacon commitment's associated block parent hash
// (which is used to calculate the commitmented random number).
func WriteRandomCommitmentCache(db ethdb.KeyValueWriter, commitment common.Hash, parentHash common.Hash) {
	if err := db.Put(istanbul.RandomnessCommitmentDBLocation(commitment), parentHash.Bytes()); err != nil {
		log.Crit("Failed to store randomness commitment cache entry", "err", err)
	}
}

// ReadRandomCommitmentCache will retun the random beacon commit's associated block parent hash.
func ReadRandomCommitmentCache(db ethdb.Reader, commitment common.Hash) common.Hash {
	parentHash, err := db.Get(istanbul.RandomnessCommitmentDBLocation(commitment))
	if err != nil {
		log.Warn("Error in trying to retrieve randomness commitment cache entry", "error", err)
		return common.Hash{}
	}

	return common.BytesToHash(parentHash)
}

// ReadAccumulatedEpochUptime retrieves the so-far accumulated uptime array for the validators of the specified epoch
func ReadAccumulatedEpochUptime(db ethdb.Reader, epoch uint64) *uptime.Uptime {
	data, _ := db.Get(uptimeKey(epoch))
	if len(data) == 0 {
		log.Trace("ReadAccumulatedEpochUptime EMPTY", "epoch", epoch)
		return nil
	}
	uptime := new(uptime.Uptime)
	if err := rlp.Decode(bytes.NewReader(data), uptime); err != nil {
		log.Error("Invalid uptime RLP", "err", err)
		return nil
	}
	return uptime
}

// WriteAccumulatedEpochUptime updates the accumulated uptime array for the validators of the specified epoch
func WriteAccumulatedEpochUptime(db ethdb.KeyValueWriter, epoch uint64, uptime *uptime.Uptime) {
	data, err := rlp.EncodeToBytes(uptime)
	if err != nil {
		log.Crit("Failed to RLP encode updated uptime", "err", err)
	}
	if err := db.Put(uptimeKey(epoch), data); err != nil {
		log.Crit("Failed to store updated uptime", "err", err)
	}
}

// uptimeKey = uptimePrefix + epoch number
func uptimeKey(epoch uint64) []byte {
	// abuse encodeBlockNumber for epochs
	return append([]byte("uptime"), encodeBlockNumber(epoch)...)
}
