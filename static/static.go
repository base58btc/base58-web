package static

var Base58 = "Base58⛓️🔓"
var IntroTitle = "Base58 is the world’s best bitcoin education company."
var Base58Pitch = "Our developer-oriented bitcoin transactions class will take you from zero to timechain hero."

// Course is a struct that holds the data for a course
type Course struct {
	Title   string
	Week    string
	Summary string
	Topics  []string
}

var introToTransactions = Course{
	Title:   "Intro to bitcoin transactions",
	Week:    "Week 1",
	Summary: "We get started by learning about what a transaction is. Specifically, what fields do they contain? We learn how to calculate a transaction id and what transaction fees are, and how are they calculated. Finally, we'll talk about coinbases and block rewards.",
	Topics:  []string{"transaction fields", "endianness", "transaction ids", "fees + transaction weights", "coinbases"},
}

var introToScript = Course{
	Title:   "Intro to Script",
	Week:    "Week 2",
	Summary: "In week two we start talking about Bitcoin's native \"programming language\": Script! We'll write our own script this week (and learn about hashes and preimages). Once we've written a script we'll try locking some bitcoins up to it, as well as unlocking them.",
	Topics:  []string{"Script", "standard scripts", "P2SH", "opcodes"},
}

var enterSegWit = Course{
	Title:   "Enter SegWit",
	Week:    "Week 3",
	Summary: "Now that we've seen how transactions are constructed and built, we'll introduce the bitcoin omnibus update bill, the SegWit soft-fork. SegWit impacted the structure of a transaction and its fee calculations, so we'll dive into how these updates work and two of the 'new' SegWit script types: P2WSH and P2SH-P2WSH.",
	Topics:  []string{"SegWit! P2WSH", "P2SH-P2WSH"},
}

var publicPrivateKeys = Course{
	Title:   "Public/Private keys; intro to elliptic curves",
	Week:    "Week 4",
	Summary: "We're halfway through class, it's about time we introduced cryptography. We'll start this week building an understanding of elliptic curves over a finite field and looking at bitcoin's secp256k1 curve parameters. Then we'll pick a private key and derive a public key. Finally we'll make our first signed transactions.",
	Topics:  []string{"elliptic curves", "secp256k1", "public/private key cryptography", "P2PK", "P2PKH"},
}

var signingTransactions = Course{
	Title:   "Signing transactions",
	Week:    "Week 5",
	Summary: "Signing transactions is actually a bit complicated. We'll talk about the ECDSA and walk through how a private key produces signatures for a transaction. We'll cover sighashes and discuss the TX_HASH proposal.",
	Topics:  []string{"ECDSA", "SegWit", "sighashes", "TX_HASH"},
}

var multisig = Course{
	Title:   "Multisig + Getting your transaction mined",
	Week:    "Week 6",
	Summary: "This is the last week of class. We'll cover multisig transactions and the OP_CHECKMULTISIG opcode. Our final class we'll cover some topics on how to get your transaction mined: RBF + CPFP.",
	Topics:  []string{"multisig", "OP_CHECKMULTISIG", "RBF", "CPFP"},
}

var Courses = []Course{
	introToTransactions,
	introToScript,
	enterSegWit,
	publicPrivateKeys,
	signingTransactions,
	multisig,
}
