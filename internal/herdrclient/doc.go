// Package herdrclient wraps every interaction this service has with the
// Herdr server behind one interface, HerdrClient. It is the single seam
// between this codebase and the untestable-live parts: resolving the
// focused pane and typing text into it both eventually shell out to the
// `herdr` CLI, a live process talking to a live Herdr server over its Unix
// socket, which a unit test cannot reasonably stand up.
//
// Production code gets ExecHerdrClient, which shells out with
// exec.CommandContext to `herdr --session <name> …` and parses its JSON
// output (all herdr CLI output is JSON — see the herdr skill). Tests get a
// fake implementation of HerdrClient instead, so /send's atomicity,
// resolution, and error-mapping logic is exercised without a live Herdr
// (see the design doc's Testing Decisions: "unit tests inject a fake
// HerdrClient and never need a live Herdr").
package herdrclient
