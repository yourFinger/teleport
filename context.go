// Copyright 2015-2017 HenryLee. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tp

import (
	"net/url"
	"reflect"
	"time"

	"github.com/henrylee2cn/goutil"

	"github.com/henrylee2cn/teleport/codec"
	"github.com/henrylee2cn/teleport/socket"
	"github.com/henrylee2cn/teleport/utils"
)

type (
	// PreCtx context method set used before reading packet header.
	PreCtx interface {
		// Peer returns the peer.
		Peer() Peer
		// Session returns the session.
		Session() Session
		// Ip returns the remote addr.
		Ip() string
		// Public returns temporary public data of context.
		Public() goutil.Map
		// PublicLen returns the length of public data of context.
		PublicLen() int
		// Rerror returns the handle error.
		Rerror() *Rerror
	}
	// BaseCtx common context method set.
	BaseCtx interface {
		PreCtx
		// Seq returns the input packet sequence.
		Seq() uint64
		// GetMeta gets the header metadata for the input packet.
		GetMeta(key string) []byte
		// CopyMeta returns the input packet metadata copy.
		CopyMeta() *utils.Args
		// Uri returns the input packet uri.
		Uri() string
		// ChangeUri changes the input packet uri.
		ChangeUri(string)
		// Url returns the input packet uri object.
		Url() *url.URL
		// Path returns the input packet uri path.
		Path() string
		// Query returns the input packet uri query object.
		Query() url.Values
	}
	// WriteCtx context method set for writing packet.
	WriteCtx interface {
		PreCtx
		// Output returns writed packet.
		Output() *socket.Packet
	}
	// ReadCtx context method set for reading packet.
	ReadCtx interface {
		BaseCtx
		// Input returns readed packet.
		Input() *socket.Packet
	}
	// PushCtx context method set for handling the pushed packet.
	// For example:
	//  type HomePush struct{ PushCtx }
	PushCtx interface {
		BaseCtx
		// GetBodyCodec gets the body codec type of the input packet.
		GetBodyCodec() byte
	}
	// PullCtx context method set for handling the pulled packet.
	// For example:
	//  type HomePull struct{ PullCtx }
	PullCtx interface {
		BaseCtx
		// Input returns readed packet.
		Input() *socket.Packet
		// GetBodyCodec gets the body codec type of the input packet.
		GetBodyCodec() byte
		// Output returns writed packet.
		Output() *socket.Packet
		// SetBodyCodec sets the body codec for reply packet.
		SetBodyCodec(byte)
		// SetMeta sets the header metadata for reply packet.
		SetMeta(key, value string)
		// AddXferPipe appends transfer filter pipe of reply packet.
		AddXferPipe(filterId ...byte)
	}
	// UnknownPushCtx context method set for handling the unknown pushed packet.
	UnknownPushCtx interface {
		BaseCtx
		// GetBodyCodec gets the body codec type of the input packet.
		GetBodyCodec() byte
		// InputBodyBytes if the input body binder is []byte type, returns it, else returns nil.
		InputBodyBytes() []byte
		// Bind when the raw body binder is []byte type, now binds the input body to v.
		Bind(v interface{}) (bodyCodec byte, err error)
	}
	// UnknownPullCtx context method set for handling the unknown pulled packet.
	UnknownPullCtx interface {
		BaseCtx
		// GetBodyCodec gets the body codec type of the input packet.
		GetBodyCodec() byte
		// InputBodyBytes if the input body binder is []byte type, returns it, else returns nil.
		InputBodyBytes() []byte
		// Bind when the raw body binder is []byte type, now binds the input body to v.
		Bind(v interface{}) (bodyCodec byte, err error)
		// SetBodyCodec sets the body codec for reply packet.
		SetBodyCodec(byte)
		// SetMeta sets the header metadata for reply packet.
		SetMeta(key, value string)
		// AddXferPipe appends transfer filter pipe of reply packet.
		AddXferPipe(filterId ...byte)
	}

	// readHandleCtx the underlying common instance of PullCtx and PushCtx.
	readHandleCtx struct {
		sess            *session
		input           *socket.Packet
		output          *socket.Packet
		handler         *Handler
		arg             reflect.Value
		pullCmd         *pullCmd
		public          goutil.Map
		start           time.Time
		cost            time.Duration
		pluginContainer *PluginContainer
		handleErr       *Rerror
		next            *readHandleCtx
	}
)

var (
	_ PreCtx         = new(readHandleCtx)
	_ BaseCtx        = new(readHandleCtx)
	_ WriteCtx       = new(readHandleCtx)
	_ ReadCtx        = new(readHandleCtx)
	_ PushCtx        = new(readHandleCtx)
	_ PullCtx        = new(readHandleCtx)
	_ UnknownPushCtx = new(readHandleCtx)
	_ UnknownPullCtx = new(readHandleCtx)
)

var (
	emptyValue  = reflect.Value{}
	emptyMethod = reflect.Method{}
)

// newReadHandleCtx creates a readHandleCtx for one request/response or push.
func newReadHandleCtx() *readHandleCtx {
	c := new(readHandleCtx)
	c.input = socket.NewPacket()
	c.input.SetNewBody(c.binding)
	c.output = socket.NewPacket()
	return c
}

func (c *readHandleCtx) reInit(s *session) {
	c.sess = s
	count := s.socket.PublicLen()
	c.public = goutil.RwMap(count)
	if count > 0 {
		s.socket.Public().Range(func(key, value interface{}) bool {
			c.public.Store(key, value)
			return true
		})
	}
}

func (c *readHandleCtx) clean() {
	c.sess = nil
	c.handler = nil
	c.arg = emptyValue
	c.pullCmd = nil
	c.public = nil
	c.cost = 0
	c.pluginContainer = nil
	c.handleErr = nil
	c.input.Reset(socket.WithNewBody(c.binding))
	c.output.Reset()
}

// Peer returns the peer.
func (c *readHandleCtx) Peer() Peer {
	return c.sess.peer
}

// Session returns the session.
func (c *readHandleCtx) Session() Session {
	return c.sess
}

// Input returns readed packet.
func (c *readHandleCtx) Input() *socket.Packet {
	return c.input
}

// Output returns writed packet.
func (c *readHandleCtx) Output() *socket.Packet {
	return c.output
}

// Public returns temporary public data of context.
func (c *readHandleCtx) Public() goutil.Map {
	return c.public
}

// PublicLen returns the length of public data of context.
func (c *readHandleCtx) PublicLen() int {
	return c.public.Len()
}

// Seq returns the input packet sequence.
func (c *readHandleCtx) Seq() uint64 {
	return c.input.Seq()
}

// Uri returns the input packet uri string.
func (c *readHandleCtx) Uri() string {
	return c.input.Uri()
}

// ChangeUri changes the input packet uri.
func (c *readHandleCtx) ChangeUri(uri string) {
	c.input.SetUri(uri)
}

// Url returns the input packet uri object.
func (c *readHandleCtx) Url() *url.URL {
	return c.input.Url()
}

// Path returns the input packet uri path.
func (c *readHandleCtx) Path() string {
	return c.input.Url().Path
}

// Query returns the input packet uri query object.
func (c *readHandleCtx) Query() url.Values {
	return c.input.Url().Query()
}

// GetMeta gets the header metadata for the input packet.
func (c *readHandleCtx) GetMeta(key string) []byte {
	return c.input.Meta().Peek(key)
}

// CopyMeta returns the input packet metadata copy.
func (c *readHandleCtx) CopyMeta() *utils.Args {
	dst := utils.AcquireArgs()
	c.input.Meta().CopyTo(dst)
	return dst
}

// SetMeta sets the header metadata for reply packet.
func (c *readHandleCtx) SetMeta(key, value string) {
	c.output.Meta().Set(key, value)
}

// GetBodyCodec gets the body codec type of the input packet.
func (c *readHandleCtx) GetBodyCodec() byte {
	return c.input.BodyCodec()
}

// SetBodyCodec sets the body codec for reply packet.
func (c *readHandleCtx) SetBodyCodec(bodyCodec byte) {
	c.output.SetBodyCodec(bodyCodec)
}

// AddXferPipe appends transfer filter pipe of reply packet.
func (c *readHandleCtx) AddXferPipe(filterId ...byte) {
	c.output.XferPipe().Append(filterId...)
}

// Ip returns the remote addr.
func (c *readHandleCtx) Ip() string {
	return c.sess.RemoteIp()
}

// Be executed synchronously when reading packet
func (c *readHandleCtx) binding(header socket.Header) (body interface{}) {
	c.start = c.sess.timeNow()
	c.pluginContainer = c.sess.peer.pluginContainer
	switch header.Ptype() {
	case TypeReply:
		return c.bindReply(header)

	case TypePush:
		return c.bindPush(header)

	case TypePull:
		return c.bindPull(header)

	default:
		c.handleErr = rerrCodeNotImplemented
		return nil
	}
}

// Be executed asynchronously after readed packet
func (c *readHandleCtx) handle() {
	if c.handleErr != nil && c.handleErr.Code == CodeNotImplemented {
		goto E
	}
	switch c.input.Ptype() {
	case TypeReply:
		// handles pull reply
		c.handleReply()
		return

	case TypePush:
		//  handles push
		c.handlePush()
		return

	case TypePull:
		// handles and replies pull
		c.handlePull()
		return

	default:
	}
E:
	// if unsupported, disconnected.
	rerrCodeNotImplemented.SetToMeta(c.output.Meta())
	if c.sess.peer.printBody {
		logformat := "disconnect(%s) due to unsupported packet type: %d |\nseq: %d |uri: %-30s |\nRECV:\n size: %d\n body[-json]: %s\n"
		Errorf(logformat, c.Ip(), c.input.Ptype(), c.input.Seq(), c.input.Uri(), c.input.Size(), bodyLogBytes(c.input))
	} else {
		logformat := "disconnect(%s) due to unsupported packet type: %d |\nseq: %d |uri: %-30s |\nRECV:\n size: %d\n"
		Errorf(logformat, c.Ip(), c.input.Ptype(), c.input.Seq(), c.input.Uri(), c.input.Size())
	}
	go c.sess.Close()
}

func (c *readHandleCtx) bindPush(header socket.Header) interface{} {
	c.handleErr = c.pluginContainer.PostReadPushHeader(c)
	if c.handleErr != nil {
		return nil
	}

	u := header.Url()
	if len(u.Path) == 0 {
		c.handleErr = rerrBadPacket.Copy()
		c.handleErr.Detail = "invalid URI for packet"
		return nil
	}

	var ok bool
	c.handler, ok = c.sess.getPushHandler(u.Path)
	if !ok {
		c.handleErr = rerrNotFound
		return nil
	}

	c.pluginContainer = c.handler.pluginContainer
	c.arg = c.handler.NewArgValue()
	c.input.SetBody(c.arg.Interface())
	c.handleErr = c.pluginContainer.PreReadPushBody(c)
	if c.handleErr != nil {
		return nil
	}

	return c.input.Body()
}

// handlePush handles push.
func (c *readHandleCtx) handlePush() {
	defer func() {
		c.cost = c.sess.timeSince(c.start)
		c.sess.runlog(c.cost, c.input, nil, typePushHandle)
	}()

	if c.handleErr == nil && c.handler != nil {
		if c.pluginContainer.PostReadPushBody(c) == nil {
			if c.handler.isUnknown {
				c.handler.unknownHandleFunc(c)
			} else {
				c.handler.handleFunc(c, c.arg)
			}
		}
	}
	if c.handleErr != nil {
		Warnf("%s", c.handleErr.String())
	}
}

func (c *readHandleCtx) bindPull(header socket.Header) interface{} {
	c.output.SetSeq(header.Seq())
	c.output.SetUri(header.Uri())

	c.handleErr = c.pluginContainer.PostReadPullHeader(c)
	if c.handleErr != nil {
		c.handleErr.SetToMeta(c.output.Meta())
		return nil
	}

	u := header.Url()
	if len(u.Path) == 0 {
		c.handleErr = rerrBadPacket.Copy()
		c.handleErr.Detail = "invalid URI for packet"
		c.handleErr.SetToMeta(c.output.Meta())
		return nil
	}

	var ok bool
	c.handler, ok = c.sess.getPullHandler(u.Path)
	if !ok {
		c.handleErr = rerrNotFound
		c.handleErr.SetToMeta(c.output.Meta())
		return nil
	}

	c.pluginContainer = c.handler.pluginContainer
	if c.handler.isUnknown {
		c.input.SetBody(new([]byte))
	} else {
		c.arg = c.handler.NewArgValue()
		c.input.SetBody(c.arg.Interface())
	}

	c.handleErr = c.pluginContainer.PreReadPullBody(c)
	if c.handleErr != nil {
		c.handleErr.SetToMeta(c.output.Meta())
		return nil
	}

	return c.input.Body()
}

// handlePull handles and replies pull.
func (c *readHandleCtx) handlePull() {
	defer func() {
		c.cost = c.sess.timeSince(c.start)
		c.sess.runlog(c.cost, c.input, c.output, typePullHandle)
	}()

	// set packet type
	c.output.SetPtype(TypeReply)

	// copy transfer filter pipe
	c.output.AppendXferPipeFrom(c.input)

	if c.handleErr == nil {
		c.handleErr = NewRerrorFromMeta(c.output.Meta())
	}

	// handle pull
	if c.handleErr == nil {
		c.handleErr = c.pluginContainer.PostReadPullBody(c)
		if c.handleErr != nil {
			c.handleErr.SetToMeta(c.output.Meta())
		} else {
			if c.handler.isUnknown {
				c.handler.unknownHandleFunc(c)
			} else {
				c.handler.handleFunc(c, c.arg)
			}
		}
	}

	// reply pull
	rerr := c.pluginContainer.PreWriteReply(c)
	if rerr == nil {
		c.handleErr = rerr
	}
	rerr = c.sess.write(c.output)
	if rerr != nil {
		if c.handleErr == nil {
			c.handleErr = rerr
		}
		rerr.SetToMeta(c.output.Meta())
	}

	rerr = c.pluginContainer.PostWriteReply(c)
	if c.handleErr == nil {
		c.handleErr = rerr
	}
}

func (c *readHandleCtx) bindReply(header socket.Header) interface{} {
	_pullCmd, ok := c.sess.pullCmdMap.Load(header.Seq())
	if !ok {
		Warnf("not found pull cmd: %v", c.input)
		return nil
	}
	c.pullCmd = _pullCmd.(*pullCmd)
	c.public = c.pullCmd.public

	rerr := c.pluginContainer.PostReadReplyHeader(c)
	if rerr != nil {
		c.pullCmd.rerr = rerr
		return nil
	}
	rerr = c.pluginContainer.PreReadReplyBody(c)
	if rerr != nil {
		c.pullCmd.rerr = rerr
		return nil
	}
	return c.pullCmd.reply
}

// handleReply handles pull reply.
func (c *readHandleCtx) handleReply() {
	if c.pullCmd == nil {
		return
	}
	defer func() {
		c.handleErr = c.pullCmd.rerr
		c.pullCmd.done()
		c.pullCmd.cost = c.sess.timeSince(c.pullCmd.start)
		c.sess.runlog(c.pullCmd.cost, c.input, c.pullCmd.output, typePullLaunch)
	}()
	if c.pullCmd.rerr != nil {
		return
	}
	rerr := NewRerrorFromMeta(c.input.Meta())
	if rerr == nil {
		rerr = c.pluginContainer.PostReadReplyBody(c)
	}
	c.pullCmd.rerr = rerr
}

// Rerror returns the handle error.
func (c *readHandleCtx) Rerror() *Rerror {
	return c.handleErr
}

// InputBodyBytes if the input body binder is []byte type, returns it, else returns nil.
func (c *readHandleCtx) InputBodyBytes() []byte {
	b, ok := c.input.Body().(*[]byte)
	if !ok {
		return nil
	}
	return *b
}

// Bind when the raw body binder is []byte type, now binds the input body to v.
func (c *readHandleCtx) Bind(v interface{}) (byte, error) {
	b := c.InputBodyBytes()
	if b == nil {
		return codec.NilCodecId, nil
	}
	c.input.SetBody(v)
	err := c.input.UnmarshalBody(b)
	return c.input.BodyCodec(), err
}

type (
	// PullCmd the command of the pulling operation's response.
	PullCmd interface {
		// Peer returns the peer.
		Peer() Peer
		// Session returns the session.
		Session() Session
		// Ip returns the remote addr.
		Ip() string
		// Public returns temporary public data of context.
		Public() goutil.Map
		// PublicLen returns the length of public data of context.
		PublicLen() int
		// Output returns writed packet.
		Output() *socket.Packet
		// Result returns the pull result.
		Result() (interface{}, *Rerror)
		// Rerror returns the pull error.
		Rerror() *Rerror
		// CostTime returns the pulled cost time.
		// If PeerConfig.CountTime=false, always returns 0.
		CostTime() time.Duration
	}
	pullCmd struct {
		sess     *session
		output   *socket.Packet
		reply    interface{}
		rerr     *Rerror
		doneChan chan PullCmd // Strobes when pull is complete.
		start    time.Time
		cost     time.Duration
		public   goutil.Map
	}
)

var _ WriteCtx = PullCmd(nil)

// Peer returns the peer.
func (p *pullCmd) Peer() Peer {
	return p.sess.peer
}

// Session returns the session.
func (p *pullCmd) Session() Session {
	return p.sess
}

// Ip returns the remote addr.
func (p *pullCmd) Ip() string {
	return p.sess.RemoteIp()
}

// Public returns temporary public data of context.
func (p *pullCmd) Public() goutil.Map {
	return p.public
}

// PublicLen returns the length of public data of context.
func (p *pullCmd) PublicLen() int {
	return p.public.Len()
}

// Output returns writed packet.
func (p *pullCmd) Output() *socket.Packet {
	return p.output
}

// Result returns the pull result.
func (p *pullCmd) Result() (interface{}, *Rerror) {
	return p.reply, p.rerr
}

// Rerror returns the pull error.
func (p *pullCmd) Rerror() *Rerror {
	return p.rerr
}

// CostTime returns the pulled cost time.
// If PeerConfig.CountTime=false, always returns 0.
func (p *pullCmd) CostTime() time.Duration {
	return p.cost
}

func (p *pullCmd) cancel() {
	p.sess.pullCmdMap.Delete(p.output.Seq())
	p.rerr = rerrConnClosed
	p.doneChan <- p
	// free count pull-launch
	p.sess.gracepullCmdWaitGroup.Done()
}

func (p *pullCmd) done() {
	p.sess.pullCmdMap.Delete(p.output.Seq())
	p.doneChan <- p
	// free count pull-launch
	p.sess.gracepullCmdWaitGroup.Done()
}
