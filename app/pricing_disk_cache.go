package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// The live on-demand price map of a region is kept on disk as well as in
// computeOnDemandCache, so a restart of meroku answers from the last load
// instead of paying for a Price List read before it has any regional price.
//
// One file per cache key (profile and region):
//
//	<user cache dir>/meroku/ec2-on-demand-prices/<key name>.json
//
// It holds {fetchedAt, region, prices}. Only maps read from AWS are written,
// never the fallback table and never an empty map. A file that cannot be read
// or parsed, or holds no prices, counts as no file.

// ec2PriceCacheDir returns the directory of the price files, or "" to turn the
// disk cache off. Tests replace it.
var ec2PriceCacheDir = defaultEC2PriceCacheDir

func defaultEC2PriceCacheDir() string {
	base, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "meroku", "ec2-on-demand-prices")
}

// ec2PriceFileMaxClockSkew is how far in the future a saved fetch time may be
// and still be trusted.
const ec2PriceFileMaxClockSkew = 5 * time.Minute

type ec2PriceFile struct {
	FetchedAt time.Time          `json:"fetchedAt"`
	Region    string             `json:"region"`
	Prices    map[string]float64 `json:"prices"`
}

// ec2PriceCacheFile is the path of key's file. The readable part keeps only
// filename-safe characters; the hash suffix keeps two keys that clean up to
// the same text apart.
func ec2PriceCacheFile(key string) (string, bool) {
	dir := ec2PriceCacheDir()
	if dir == "" {
		return "", false
	}
	readable := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '.':
			return r
		}
		return '_'
	}, key)
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(dir, readable+"-"+hex.EncodeToString(sum[:4])+".json"), true
}

// ec2PriceCacheLogged keeps each unusable-file message to once per path.
var ec2PriceCacheLogged sync.Map

func logEC2PriceCacheOnce(path string, err error) {
	if _, seen := ec2PriceCacheLogged.LoadOrStore(path, true); !seen {
		log.Printf("[Pricing] ignoring EC2 price cache file %s: %v", path, err)
	}
}

// saveLiveEC2Prices writes prices for key, replacing the file in one rename so
// a reader never sees half a file. Failures are logged and otherwise ignored:
// the memory cache still has the prices.
func saveLiveEC2Prices(key, region string, prices map[string]float64, fetchedAt time.Time) {
	if len(prices) == 0 {
		return
	}
	path, ok := ec2PriceCacheFile(key)
	if !ok {
		return
	}
	if err := writeEC2PriceFile(path, ec2PriceFile{FetchedAt: fetchedAt.UTC(), Region: region, Prices: prices}); err != nil {
		log.Printf("[Pricing] saving EC2 prices for %s: %v", region, err)
	}
}

func writeEC2PriceFile(path string, file ec2PriceFile) error {
	body, err := json.Marshal(file)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".prices-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // a no-op once the rename has happened
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// readEC2PriceFile returns the prices saved for key. A missing file is silent;
// any other unusable file is logged once.
func readEC2PriceFile(key string) (ec2PriceFile, bool) {
	path, ok := ec2PriceCacheFile(key)
	if !ok {
		return ec2PriceFile{}, false
	}
	body, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			logEC2PriceCacheOnce(path, err)
		}
		return ec2PriceFile{}, false
	}
	var file ec2PriceFile
	if err := json.Unmarshal(body, &file); err != nil {
		logEC2PriceCacheOnce(path, err)
		return ec2PriceFile{}, false
	}
	if len(file.Prices) == 0 || file.FetchedAt.IsZero() {
		logEC2PriceCacheOnce(path, errors.New("no prices or no fetch time"))
		return ec2PriceFile{}, false
	}
	// A fetch time in the future would read as fresh until the clock caught
	// up with it. Beyond ordinary clock skew the file is not trusted.
	if file.FetchedAt.After(time.Now().Add(ec2PriceFileMaxClockSkew)) {
		logEC2PriceCacheOnce(path, fmt.Errorf("fetchedAt %s is in the future", file.FetchedAt.Format(time.RFC3339)))
		return ec2PriceFile{}, false
	}
	return file, true
}

// peekOnDemandPrices is computeOnDemandCache.Peek with the disk behind it:
// when memory holds nothing for key, the saved file is loaded into memory with
// its original fetch time first. Every read of the live on-demand map goes
// through here, so pool pricing and the instance picker see the same entry.
// Like Peek it never starts a fetch.
func peekOnDemandPrices(key string) (map[string]float64, time.Time, bool) {
	if prices, at, ok := computeOnDemandCache.Peek(key); ok {
		return prices, at, true
	}
	if file, ok := readEC2PriceFile(key); ok {
		computeOnDemandCache.SeedIfAbsent(key, file.Prices, file.FetchedAt)
	}
	return computeOnDemandCache.Peek(key)
}
