package testutil

// PurchaseIntent is a dependency-neutral typed fixture shared by operation tests.
// Provider behavior itself lives with the operations tests so testutil remains a
// leaf package and cannot create import cycles in other package test suites.
type PurchaseIntent struct {
	SKU      string `json:"sku"`
	Quantity int    `json:"quantity"`
}

func (PurchaseIntent) DescriptorType() string { return "purchase" }
