package postgres_test

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/papercomputeco/tapes/pkg/storage"
	"github.com/papercomputeco/tapes/pkg/storage/postgres"
)

var _ = Describe("raw turn detail [postgres]", func() {
	const (
		harnessID        = "pi"
		harnessSessionID = "5b0d5e66-5b4a-4f2a-9d0c-2a4a6a7f1c11"
		otherSessionID   = "9c1e7f02-3d4b-4c5a-8e6f-0a1b2c3d4e5f"
	)

	var (
		driver *postgres.Driver
		ctx    context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()

		d, err := postgres.NewDriver(ctx, testPostgresDSN)
		Expect(err).NotTo(HaveOccurred())
		driver = d

		_, err = driver.DB().Exec(ctx, "TRUNCATE TABLE raw_turns RESTART IDENTITY")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if driver != nil {
			driver.Close()
		}
	})

	put := func(session, requestID, request string) {
		GinkgoHelper()
		inserted, err := driver.PutRawTurn(ctx, storage.RawTurnRecord{
			Source:           storage.RawTurnSourceWire,
			Provider:         "openai",
			AgentName:        "pi",
			HarnessID:        harnessID,
			HarnessSessionID: session,
			RequestID:        requestID,
			RawRequest:       json.RawMessage(request),
			Response:         json.RawMessage(`{"model":"m","reply":"` + requestID + `"}`),
			Meta:             json.RawMessage(`{"request_id":"` + requestID + `","request_bytes":321,"response_bytes":45}`),
			SessionEnvelope:  json.RawMessage(`{"harness_id":"` + harnessID + `","harness_session_id":"` + session + `"}`),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(inserted).To(BeTrue())
	}

	It("returns one turn's payloads with the header the listing reports", func() {
		put(harnessSessionID, "req-1", `{"model":"m","messages":[{"role":"user","content":"first"}]}`)
		put(harnessSessionID, "req-2", `{"model":"m","messages":[{"role":"user","content":"second"}],"unmodeled":{"kept":true}}`)

		headers, err := driver.ListRawTurnHeaders(ctx, "", harnessID, harnessSessionID, 0, 10)
		Expect(err).NotTo(HaveOccurred())
		Expect(headers).To(HaveLen(2))

		turn, err := driver.GetRawTurn(ctx, "", harnessID, harnessSessionID, headers[1].ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(turn).NotTo(BeNil())
		Expect(turn.RawTurnHeader).To(Equal(headers[1]))
		Expect(turn.RawRequest).To(MatchJSON(`{"model":"m","messages":[{"role":"user","content":"second"}],"unmodeled":{"kept":true}}`))
		Expect(turn.Response).To(MatchJSON(`{"model":"m","reply":"req-2"}`))
	})

	It("returns nil for a turn of another session or an unknown id", func() {
		put(harnessSessionID, "req-mine", `{"model":"m"}`)
		put(otherSessionID, "req-theirs", `{"model":"m"}`)

		theirs, err := driver.ListRawTurnHeaders(ctx, "", harnessID, otherSessionID, 0, 10)
		Expect(err).NotTo(HaveOccurred())
		Expect(theirs).To(HaveLen(1))

		turn, err := driver.GetRawTurn(ctx, "", harnessID, harnessSessionID, theirs[0].ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(turn).To(BeNil())

		turn, err = driver.GetRawTurn(ctx, "", harnessID, harnessSessionID, 9999)
		Expect(err).NotTo(HaveOccurred())
		Expect(turn).To(BeNil())
	})
})
