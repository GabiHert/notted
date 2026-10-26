package askipc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"

	"github.com/GabiHert/notted/internal/application/port"
	"github.com/GabiHert/notted/internal/domain"
)

// Client sends forms to the running app for one meeting.
type Client struct{ Socket, MeetingID, Token string }

var _ port.FormAsker = Client{}

// Ask sends form to the app and waits for the answer.
func (c Client) Ask(ctx context.Context, kind domain.FormKind, form domain.Form) (domain.FormAnswer, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", c.Socket)
	if err != nil {
		return domain.FormAnswer{}, fmt.Errorf("%w: %v", domain.ErrAppNotRunning, err)
	}
	defer conn.Close()

	rawForm, err := encodeLine(form)
	if err != nil {
		return domain.FormAnswer{}, err
	}
	line, err := encodeLine(request{V: protocolVersion, Token: c.Token, MeetingID: c.MeetingID, Kind: string(kind), Form: bytes.TrimSpace(rawForm)})
	if err != nil {
		return domain.FormAnswer{}, err
	}
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-stop:
		}
	}()
	if _, err := conn.Write(line); err != nil {
		if ctx.Err() != nil {
			return domain.FormAnswer{}, ctx.Err()
		}
		return domain.FormAnswer{}, errors.New("Notted stopped before the form was answered.")
	}
	raw, err := readLine(bufio.NewReaderSize(conn, 64<<10), maxReplyBytes)
	if ctx.Err() != nil {
		return domain.FormAnswer{}, ctx.Err()
	}
	if err != nil {
		return domain.FormAnswer{}, errors.New("Notted stopped before the form was answered.")
	}
	var rep reply
	if err := json.Unmarshal(raw, &rep); err != nil {
		return domain.FormAnswer{}, fmt.Errorf("Notted sent a reply that could not be read: %v", err)
	}
	if !rep.OK || rep.Answer == nil {
		return domain.FormAnswer{}, errors.New(rep.Error)
	}
	ans := *rep.Answer
	if ans.Answers == nil {
		ans.Answers = map[string]any{}
	}
	return ans, nil
}
