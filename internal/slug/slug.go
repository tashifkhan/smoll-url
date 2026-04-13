package slug

import (
	"crypto/rand"
	"math/big"
)

var adjectives = []string{
	"admiring", "adoring", "amazing", "brave", "calm", "clever", "cool", "curious", "daring", "dazzling",
	"eager", "elegant", "epic", "festive", "focused", "friendly", "gifted", "graceful", "happy", "hopeful",
	"intrepid", "jolly", "keen", "kind", "lucid", "mystic", "nifty", "peaceful", "priceless", "quirky",
	"relaxed", "serene", "sharp", "stoic", "trusting", "upbeat", "vibrant", "vigorous", "wizardly", "zealous",
}

var names = []string{
	"agnesi", "aryabhata", "babbage", "banach", "bohr", "bose", "curie", "darwin", "dijkstra", "einstein",
	"euler", "faraday", "fermat", "feynman", "franklin", "gauss", "germain", "goldberg", "goodall", "hawking",
	"hopper", "hypatia", "johnson", "kalam", "kepler", "knuth", "lamport", "liskov", "lovelace", "maxwell",
	"mccarthy", "mendeleev", "mirzakhani", "newton", "noether", "raman", "ramanujan", "ritchie", "shannon", "turing",
}

const smallChars = "abcdefghijklmnopqrstuvwxyz0123456789"
const mixedChars = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz123456789"

func Generate(style string, length int, allowCapitalLetters bool) string {
	if style == "UID" {
		if allowCapitalLetters {
			return randomFromAlphabet(mixedChars, length)
		}
		return randomFromAlphabet(smallChars, length)
	}

	return randomChoice(adjectives) + "-" + randomChoice(names)
}

func randomChoice(items []string) string {
	if len(items) == 0 {
		return ""
	}
	idx := randomInt(len(items))
	return items[idx]
}

func randomFromAlphabet(alphabet string, length int) string {
	if length <= 0 {
		return ""
	}
	b := make([]byte, length)
	for i := range b {
		b[i] = alphabet[randomInt(len(alphabet))]
	}
	return string(b)
}

func randomInt(max int) int {
	if max <= 1 {
		return 0
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0
	}
	return int(n.Int64())
}
