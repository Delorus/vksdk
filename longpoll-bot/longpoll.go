/*
Package longpoll implements Bots Long Poll API.

See more https://vk.com/dev/bots_longpoll
*/
package longpoll // import "github.com/SevereCloud/vksdk/longpoll-bot"

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"

	"github.com/SevereCloud/vksdk/internal"

	"github.com/SevereCloud/vksdk/api"
	"github.com/SevereCloud/vksdk/events"
)

// Response struct.
type Response struct {
	Ts      string              `json:"ts"`
	Updates []events.GroupEvent `json:"updates"`
	Failed  int                 `json:"failed"`
}

// Longpoll struct.
type Longpoll struct {
	GroupID int
	Server  string
	Key     string
	Ts      string
	Wait    int
	VK      *api.VK
	Client  *http.Client

	funcFullResponseList []func(Response)
	inShutdown           int32

	events.FuncList
}

// NewLongpoll returns a new Longpoll.
//
// The Longpoll will use the http.DefaultClient.
// This means that if the http.DefaultClient is modified by other components
// of your application the modifications will be picked up by the SDK as well.
func NewLongpoll(vk *api.VK, groupID int) (*Longpoll, error) {
	lp := &Longpoll{
		VK:      vk,
		GroupID: groupID,
		Wait:    25,
		Client:  http.DefaultClient,
	}
	lp.FuncList = *events.NewFuncList()

	err := lp.updateServer(true)

	return lp, err
}

// NewLongpollCommunity returns a new Longpoll for community token.
//
// The Longpoll will use the http.DefaultClient.
// This means that if the http.DefaultClient is modified by other components
// of your application the modifications will be picked up by the SDK as well.
func NewLongpollCommunity(vk *api.VK) (*Longpoll, error) {
	resp, err := vk.GroupsGetByID(api.Params{})
	if err != nil {
		return nil, err
	}

	lp := &Longpoll{
		VK:      vk,
		GroupID: resp[0].ID,
		Wait:    25,
		Client:  http.DefaultClient,
	}
	lp.FuncList = *events.NewFuncList()

	err = lp.updateServer(true)

	return lp, err
}

func (lp *Longpoll) updateServer(updateTs bool) error {
	params := api.Params{
		"group_id": lp.GroupID,
	}

	serverSetting, err := lp.VK.GroupsGetLongPollServer(params)
	if err != nil {
		return err
	}

	lp.Key = serverSetting.Key
	lp.Server = serverSetting.Server

	if updateTs {
		lp.Ts = serverSetting.Ts
	}

	return nil
}

func (lp *Longpoll) check() (Response, error) {
	var response Response

	u := fmt.Sprintf("%s?act=a_check&key=%s&ts=%s&wait=%d", lp.Server, lp.Key, lp.Ts, lp.Wait)

	resp, err := lp.Client.Get(u)
	if err != nil {
		return response, err
	}
	defer resp.Body.Close()

	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return response, err
	}

	err = lp.checkResponse(response)

	return response, err
}

func (lp *Longpoll) checkResponse(response Response) (err error) {
	switch response.Failed {
	case 0:
		lp.Ts = response.Ts
	case 1:
		lp.Ts = response.Ts
	case 2:
		err = lp.updateServer(false)
	case 3:
		err = lp.updateServer(true)
	default:
		err = &Failed{response.Failed}
	}

	return
}

// Run handler.
func (lp *Longpoll) Run() error {
	atomic.StoreInt32(&lp.inShutdown, 0)

	for atomic.LoadInt32(&lp.inShutdown) == 0 {
		resp, err := lp.check()
		if err != nil {
			return err
		}

		ctx := context.WithValue(context.Background(), internal.LongpollTsKey, resp.Ts)

		for _, event := range resp.Updates {
			err = lp.Handler(ctx, event)
			if err != nil {
				return err
			}
		}

		for _, f := range lp.funcFullResponseList {
			f(resp)
		}
	}

	return nil
}

// Shutdown gracefully shuts down the longpoll without interrupting any active connections.
func (lp *Longpoll) Shutdown() {
	atomic.StoreInt32(&lp.inShutdown, 1)
}

// FullResponse handler.
func (lp *Longpoll) FullResponse(f func(Response)) {
	lp.funcFullResponseList = append(lp.funcFullResponseList, f)
}
