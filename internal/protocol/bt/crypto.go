package bt

import (
	"strings"

	torrentlib "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/mse"
)

// CryptoOptions 映射 aria2 BT 加密相关配置。
type CryptoOptions struct {
	ForceEncryption bool
	RequireCrypto   bool
	MinCryptoLevel  string
}

func applyBTCryptoOptions(cfg *torrentlib.ClientConfig, opts CryptoOptions) {
	if cfg == nil {
		return
	}

	level := strings.ToLower(strings.TrimSpace(opts.MinCryptoLevel))
	if level == "" {
		level = "plain"
	}

	switch {
	case level == "full" || level == "arc4" || opts.ForceEncryption:
		cfg.CryptoProvides = mse.CryptoMethodRC4
		cfg.HeaderObfuscationPolicy = torrentlib.HeaderObfuscationPolicy{
			Preferred:        true,
			RequirePreferred: true,
		}
		cfg.CryptoSelector = func(provided mse.CryptoMethod) mse.CryptoMethod {
			if provided&mse.CryptoMethodRC4 != 0 {
				return mse.CryptoMethodRC4
			}
			return 0
		}
	case level == "handshake" || opts.RequireCrypto:
		cfg.HeaderObfuscationPolicy.Preferred = true
		cfg.HeaderObfuscationPolicy.RequirePreferred = true
	}
}
