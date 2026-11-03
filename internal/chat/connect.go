package chat


// ConnectBlocks is the A2UI the server attaches when a provider's CLI can't answer: how to
// connect the user's OWN subscription (Account → AI subscriptions runs the vendor's sign-in in
// their terminal container). Nothing here asks for or carries a credential.
func ConnectBlocks(name string) []Block {
	raw := []Block{
		{Type: "card", Title: "Connect your " + name + " subscription",
			Body: "Chat answers with the AI CLI in your workspace, signed in with your own subscription. It isn't signed in yet.",
			Items: []Block{{Type: "button", Label: "Connect subscription", Href: "/account/ai"}}},
		{Type: "modal", Title: "How connecting works", Label: "How it works",
			Body: "1. Open Account → AI subscriptions and choose " + name + ".\n2. A terminal opens in your own container and runs the vendor's sign-in.\n3. Finish the sign-in in your browser. The credential stays in your container; Shippin never sees it.\n4. Come back and send your message again.",
			Items: []Block{{Type: "button", Label: "Open AI subscriptions", Href: "/account/ai"}}},
	}
	out := []Block{}
	for _, b := range raw {
		if v, ok := Validate(b, 0); ok {
			out = append(out, v)
		}
	}
	return out
}
