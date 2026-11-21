package handlers

import (
	"encoding/gob"
	"sync"

	"github.com/kodylow/base58-website/checkout"
)

// CourseItem keeps its original gob name for carts embedded in old sessions.
// There are deliberately no payment callbacks on this data type.
type CourseItem struct {
	checkout.Item
}

var legacyCartTypesOnce sync.Once

func RegisterLegacyCartTypes() {
	legacyCartTypesOnce.Do(func() {
		checkout.RegisterCartSerialization()
		gob.Register(CourseItem{})
	})
}
