package fed

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

// WebRTC audio framing.
//
// The data channel carries a request/response protocol that mirrors the HTTP
// semantics of the existing yamux-direct path (relay.go directResolver) so Range
// seeking and the four preserved headers pass through unchanged.
//
// Wire format (all integers are big-endian uint32, framing the bytes that follow):
//
//	request  = lenHdr(1) u32, audioRequest JSON
//	response = lenHdr(1) u32, audioResponse JSON, then body bytes to EOF
//
// The answering peer serializes its local /api/tracks/{id}/audio response into
// this frame and copies the body. The offerer reads the response header, writes
// the status + headers to the http.ResponseWriter, and streams the body.

// audioRequest is the offerer's ask for a track's audio bytes.
type audioRequest struct {
	TrackID int64  `json:"track_id"`
	Range   string `json:"range,omitempty"` // raw Range header value, or empty
}

// audioResponse is the answerer's status + preserved headers. The body follows.
type audioResponse struct {
	Status       int               `json:"status"`
	Headers      map[string]string `json:"headers"` // Content-Type/Length/Range/Accept-Ranges
	ErrorMessage string            `json:"error,omitempty"`
}

// writeFrame writes a length-prefixed JSON payload to w.
func writeFrame(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(b)))
	if _, err := w.Write(lenBuf[:]); err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

// readFrame reads a length-prefixed JSON payload from r into v.
func readFrame(r io.Reader, v any) error {
	var lenBuf [4]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return err
	}
	n := binary.BigEndian.Uint32(lenBuf[:])
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}
	return json.Unmarshal(buf, v)
}

// ServeAudioOverConn is the answerer side: it loops reading audioRequests from
// the data channel, serves each against the local app handler (which dispatches
// to trackAudio → http.ServeFile for local tracks), and writes the framed
// response + body back. It returns when the channel closes or a frame is
// malformed. appHandler must be the same handler serving /api/tracks/{id}/audio
// locally (i.e. the PeerRoutes app handler).
func ServeAudioOverConn(conn io.ReadWriter, appHandler http.Handler, peerBasePath string) error {
	for {
		var req audioRequest
		if err := readFrame(conn, &req); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if err := serveOneOverConn(conn, appHandler, peerBasePath, req); err != nil {
			return err
		}
	}
}

func serveOneOverConn(conn io.ReadWriter, appHandler http.Handler, peerBasePath string, req audioRequest) error {
	url := peerBasePath + "/api/tracks/" + strconv.FormatInt(req.TrackID, 10) + "/audio"
	httpReq, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		_ = writeFrame(conn, audioResponse{Status: http.StatusInternalServerError, ErrorMessage: "bad request"})
		return nil
	}
	if req.Range != "" {
		httpReq.Header.Set("Range", req.Range)
	}
	rec := newCaptureRecorder()
	appHandler.ServeHTTP(rec, httpReq)

	resp := audioResponse{Status: rec.code, Headers: make(map[string]string)}
	for _, k := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges"} {
		if v := rec.header().Get(k); v != "" {
			resp.Headers[k] = v
		}
	}
	if err := writeFrame(conn, resp); err != nil {
		return err
	}
	_, err = conn.Write(rec.body.Bytes())
	return err
}

// serveAudioRequest is the offerer side: send a request frame, read the response
// frame, write status + headers to w, then stream the body. remoteBodyLen (from
// Content-Length when present) bounds the copy; otherwise it copies until the
// data channel signals the response is complete — but since a single request
// occupies the channel, we rely on Content-Length to delimit the body.
func serveAudioRequest(conn io.ReadWriter, w http.ResponseWriter, remoteID int64, rangeHeader string) error {
	if err := writeFrame(conn, audioRequest{TrackID: remoteID, Range: rangeHeader}); err != nil {
		return fmt.Errorf("write request: %w", err)
	}
	var resp audioResponse
	if err := readFrame(conn, &resp); err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	for k, v := range resp.Headers {
		w.Header().Set(k, v)
	}
	w.WriteHeader(resp.Status)
	// The body length is the Content-Length the answerer forwarded. Copy exactly
	// that many bytes so we don't consume the next frame.
	clen := -1
	if v, ok := resp.Headers["Content-Length"]; ok {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			clen = int(n)
		}
	}
	if clen >= 0 {
		if _, err := io.CopyN(w, conn, int64(clen)); err != nil {
			return err
		}
	} else {
		// No Content-Length: best-effort copy of the rest of the channel. In
		// practice audio responses always carry Content-Length (http.ServeFile
		// sets it), so this branch is a defensive fallback.
		_, _ = io.Copy(w, conn)
	}
	return nil
}

// captureRecorder is an httptest-style ResponseWriter that buffers the body and
// headers so ServeAudioOverConn can re-frame the local handler's response. It
// does not support streaming flushes beyond buffering the full body, which is
// acceptable for audio files served by http.ServeFile.
type captureRecorder struct {
	headerMap http.Header
	body      limitedBuffer
	code      int
	wrote     bool
}

func newCaptureRecorder() *captureRecorder {
	return &captureRecorder{headerMap: make(http.Header)}
}

func (c *captureRecorder) Header() http.Header { return c.headerMap }

// header returns the header map (helper for callers that need it before Write).
func (c *captureRecorder) header() http.Header { return c.headerMap }

func (c *captureRecorder) WriteHeader(code int) {
	if c.wrote {
		return
	}
	c.code = code
	c.wrote = true
}

func (c *captureRecorder) Write(p []byte) (int, error) {
	if !c.wrote {
		c.WriteHeader(http.StatusOK)
	}
	return c.body.Write(p)
}

// limitedBuffer is a bytes.Buffer without the import; grows as needed.
type limitedBuffer struct {
	data []byte
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *limitedBuffer) Bytes() []byte { return b.data }
