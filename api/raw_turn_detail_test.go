package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	tapeslogger "github.com/papercomputeco/tapes/pkg/logger"
	"github.com/papercomputeco/tapes/pkg/storage"
	"github.com/papercomputeco/tapes/pkg/storage/inmemory"
)

// rawTurnDetailStub serves one session and the raw turns stored for it.
type rawTurnDetailStub struct {
	storage.Driver
	sessionsReader

	session storage.SessionRecord
	turns   []storage.RawTurnDetail
}

func (d *rawTurnDetailStub) GetSessionRecord(_ context.Context, _, id string) (*storage.SessionRecord, error) {
	if id != d.session.ID {
		return nil, nil
	}
	sess := d.session
	return &sess, nil
}

func (d *rawTurnDetailStub) GetRawTurn(_ context.Context, _, harnessID, harnessSessionID string, id int64) (*storage.RawTurnDetail, error) {
	if harnessID != d.session.HarnessID || harnessSessionID != d.session.HarnessSessionID {
		return nil, nil
	}
	for _, t := range d.turns {
		if t.ID == id {
			return &t, nil
		}
	}
	return nil, nil
}

var _ = Describe("GET /v1/sessions/{id}/raw_turns/{raw_turn_id}", func() {
	const sessionID = "7a1d2c3e-4b5f-4a6b-8c7d-9e0f1a2b3c4d"
	const request = `{"model":"m","messages":[{"role":"user","content":"hi"}],"unmodeled":{"kept":true}}`
	const response = `{"model":"m","message":{"role":"assistant","content":"hello"}}`

	session := storage.SessionRecord{
		ID:               sessionID,
		HarnessID:        "pi",
		HarnessSessionID: "harness-session-1",
	}

	turns := []storage.RawTurnDetail{
		{
			RawTurnHeader: storage.RawTurnHeader{
				ID:           7,
				Source:       storage.RawTurnSourceWire,
				Provider:     "openai",
				RequestID:    "req-7",
				ReceivedAt:   time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC),
				Meta:         json.RawMessage(`{"request_bytes":84}`),
				RequestBytes: 84,
			},
			RawRequest: json.RawMessage(request),
			Response:   json.RawMessage(response),
		},
		{RawTurnHeader: storage.RawTurnHeader{ID: 8, Source: storage.RawTurnSourceTranscript}},
	}

	get := func(driver storage.Driver, path string) (int, []byte) {
		GinkgoHelper()
		server, err := NewServer(Config{ListenAddr: ":0"}, driver, tapeslogger.NewNoop())
		Expect(err).NotTo(HaveOccurred())
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, path, nil)
		Expect(err).NotTo(HaveOccurred())
		resp, err := server.app.Test(req)
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		return resp.StatusCode, body
	}

	stub := func() storage.Driver {
		return &rawTurnDetailStub{Driver: inmemory.NewDriver(), session: session, turns: turns}
	}

	It("returns the header with the request and response verbatim", func() {
		status, body := get(stub(), "/v1/sessions/"+sessionID+"/raw_turns/7")
		Expect(status).To(Equal(http.StatusOK), string(body))

		var item RawTurnDetailItem
		Expect(json.Unmarshal(body, &item)).To(Succeed(), string(body))
		Expect(item.ID).To(Equal(int64(7)))
		Expect(item.Provider).To(Equal("openai"))
		Expect(item.RequestID).To(Equal("req-7"))
		Expect(item.RequestBytes).To(Equal(int64(84)))
		Expect(string(item.Request)).To(Equal(request))
		Expect(string(item.Response)).To(Equal(response))
	})

	It("serves null for payloads the turn never stored", func() {
		status, body := get(stub(), "/v1/sessions/"+sessionID+"/raw_turns/8")
		Expect(status).To(Equal(http.StatusOK), string(body))

		var fields map[string]json.RawMessage
		Expect(json.Unmarshal(body, &fields)).To(Succeed(), string(body))
		Expect(string(fields["request"])).To(Equal("null"))
		Expect(string(fields["response"])).To(Equal("null"))
	})

	It("answers 404 for a raw turn the session does not have", func() {
		status, _ := get(stub(), "/v1/sessions/"+sessionID+"/raw_turns/99")
		Expect(status).To(Equal(http.StatusNotFound))
	})

	It("answers 404 for an unknown session", func() {
		status, _ := get(stub(), "/v1/sessions/0f9e8d7c-6b5a-4493-8271-605f4e3d2c1b/raw_turns/7")
		Expect(status).To(Equal(http.StatusNotFound))
	})

	It("rejects a malformed session id or raw turn id", func() {
		for _, path := range []string{
			"/v1/sessions/not-a-uuid/raw_turns/7",
			"/v1/sessions/" + sessionID + "/raw_turns/seven",
			"/v1/sessions/" + sessionID + "/raw_turns/0",
			"/v1/sessions/" + sessionID + "/raw_turns/-3",
		} {
			status, body := get(stub(), path)
			Expect(status).To(Equal(http.StatusBadRequest), path+": "+string(body))
		}
	})

	It("answers 501 on a backend without the raw layer", func() {
		driver := &rawTurnHeaderStub{Driver: inmemory.NewDriver(), session: storage.SessionRecord{
			ID: sessionID, HarnessID: session.HarnessID, HarnessSessionID: session.HarnessSessionID,
		}}
		status, _ := get(driver, "/v1/sessions/"+sessionID+"/raw_turns/7")
		Expect(status).To(Equal(http.StatusNotImplemented))
	})
})
