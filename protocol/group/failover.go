package group

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/adapter/outbound"
	"github.com/sagernet/sing-box/common/interrupt"
	"github.com/sagernet/sing-box/common/urltest"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/experimental/libbox/platform"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/service"
)

func RegisterFailover(registry *outbound.Registry) {
	outbound.Register[option.FailoverOutboundOptions](registry, C.TypeFailover, NewFailover)
}

var (
	_ adapter.OutboundGroup             = (*Failover)(nil)
	_ adapter.ConnectionHandlerEx       = (*Failover)(nil)
	_ adapter.PacketConnectionHandlerEx = (*Failover)(nil)
)

type Failover struct {
	outbound.Adapter
	ctx                          context.Context
	cancel                       context.CancelFunc // 🔥 新增：用于取消 context
	outboundManager              adapter.OutboundManager
	connection                   adapter.ConnectionManager
	logger                       log.ContextLogger
	tags                         []string
	maxFailures                  int
	recoveryInterval             time.Duration
	recoveryURL                  string
	outbounds                    []adapter.Outbound
	selected                     atomic.Int32
	consecutiveFailures          []atomic.Int32
	interruptGroup               *interrupt.Group
	interruptExternalConnections bool
	access                       sync.Mutex
	recoveryTicker               *time.Ticker
	close                        chan struct{}
	closeOnce                    sync.Once // 🔥 新增：确保只关闭一次
	started                      bool
	wg                           sync.WaitGroup // 🔥 新增：等待 goroutine 退出
}

func NewFailover(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.FailoverOutboundOptions) (adapter.Outbound, error) {
	if len(options.Outbounds) == 0 {
		return nil, E.New("missing outbounds")
	}

	maxFailures := options.MaxFailures
	if maxFailures == 0 {
		maxFailures = 3
	}

	recoveryInterval := time.Duration(options.RecoveryInterval)
	if recoveryInterval == 0 {
		recoveryInterval = 5 * time.Minute
	}

	// 🔥 创建带取消的 context
	ctx, cancel := context.WithCancel(ctx)

	return &Failover{
		Adapter:                      outbound.NewAdapter(C.TypeFailover, tag, nil, options.Outbounds),
		ctx:                          ctx,
		cancel:                       cancel,
		outboundManager:              service.FromContext[adapter.OutboundManager](ctx),
		connection:                   service.FromContext[adapter.ConnectionManager](ctx),
		logger:                       logger,
		tags:                         options.Outbounds,
		maxFailures:                  maxFailures,
		recoveryInterval:             recoveryInterval,
		recoveryURL:                  options.RecoveryURL,
		consecutiveFailures:          make([]atomic.Int32, len(options.Outbounds)),
		interruptGroup:               interrupt.NewGroup(),
		interruptExternalConnections: options.InterruptExistConnections,
		close:                        make(chan struct{}),
	}, nil
}

func (f *Failover) Network() []string {
	selected := f.getSelected()
	if selected == nil {
		return []string{N.NetworkTCP, N.NetworkUDP}
	}
	return selected.Network()
}

func (f *Failover) Start() error {
	f.outbounds = make([]adapter.Outbound, 0, len(f.tags))
	for i, tag := range f.tags {
		detour, loaded := f.outboundManager.Outbound(tag)
		if !loaded {
			return E.New("outbound ", i, " not found: ", tag)
		}
		f.outbounds = append(f.outbounds, detour)
	}

	f.selected.Store(0)
	f.started = true

	// 🔥 启动主节点恢复检测（使用 WaitGroup 跟踪）
	f.recoveryTicker = time.NewTicker(f.recoveryInterval)
	f.wg.Add(1)
	go f.recoveryCheckLoop()

	f.logger.Info("failover started with ", len(f.outbounds), " outbounds, primary: ", f.tags[0])
	return nil
}

func (f *Failover) Close() error {
	f.closeOnce.Do(func() {
		// 🔥 1. 取消 context，通知所有操作停止
		if f.cancel != nil {
			f.cancel()
		}

		// 🔥 2. 关闭 close channel
		close(f.close)

		// 🔥 3. 停止 ticker
		f.access.Lock()
		if f.recoveryTicker != nil {
			f.recoveryTicker.Stop()
		}
		f.access.Unlock()

		// 🔥 4. 等待 goroutine 退出（最多等待 3 秒）
		done := make(chan struct{})
		go func() {
			f.wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			f.logger.Debug("failover goroutine exited cleanly")
		case <-time.After(3 * time.Second):
			f.logger.Warn("failover goroutine exit timeout")
		}
	})
	return nil
}

func (f *Failover) Now() string {
	selected := f.getSelected()
	if selected == nil {
		return f.tags[0]
	}
	return selected.Tag()
}

func (f *Failover) All() []string {
	return f.tags
}

func (f *Failover) getSelected() adapter.Outbound {
	idx := int(f.selected.Load())
	if idx >= 0 && idx < len(f.outbounds) {
		return f.outbounds[idx]
	}
	return nil
}

// 🔥 核心方法：真实连接时检测失败
func (f *Failover) DialContext(ctx context.Context, network string, destination M.Socksaddr) (net.Conn, error) {
	idx := int(f.selected.Load())
	if idx >= len(f.outbounds) {
		return nil, E.New("no available outbound")
	}
	selected := f.outbounds[idx]

	conn, err := selected.DialContext(ctx, network, destination)
	if err != nil {
		// 连接失败，增加连续失败计数
		failures := f.consecutiveFailures[idx].Add(1)
		f.logger.Warn("outbound ", selected.Tag(), " dial failed (", failures, "/", f.maxFailures, "): ", err)

		if int(failures) >= f.maxFailures {
			// 达到阈值，切换到下一个节点
			f.switchToNext(idx)

			// 用新节点重试本次连接
			newIdx := int(f.selected.Load())
			if newIdx != idx && newIdx < len(f.outbounds) {
				newSelected := f.outbounds[newIdx]
				conn, err = newSelected.DialContext(ctx, network, destination)
				if err == nil {
					f.consecutiveFailures[newIdx].Store(0)
					return f.interruptGroup.NewConn(conn, interrupt.IsExternalConnectionFromContext(ctx)), nil
				}
			}
		}
		return nil, err
	}

	// 连接成功，重置当前节点的失败计数
	f.consecutiveFailures[idx].Store(0)
	return f.interruptGroup.NewConn(conn, interrupt.IsExternalConnectionFromContext(ctx)), nil
}

func (f *Failover) ListenPacket(ctx context.Context, destination M.Socksaddr) (net.PacketConn, error) {
	idx := int(f.selected.Load())
	if idx >= len(f.outbounds) {
		return nil, E.New("no available outbound")
	}
	selected := f.outbounds[idx]

	conn, err := selected.ListenPacket(ctx, destination)
	if err != nil {
		failures := f.consecutiveFailures[idx].Add(1)
		f.logger.Warn("outbound ", selected.Tag(), " listen packet failed (", failures, "/", f.maxFailures, "): ", err)

		if int(failures) >= f.maxFailures {
			f.switchToNext(idx)

			newIdx := int(f.selected.Load())
			if newIdx != idx && newIdx < len(f.outbounds) {
				newSelected := f.outbounds[newIdx]
				conn, err = newSelected.ListenPacket(ctx, destination)
				if err == nil {
					f.consecutiveFailures[newIdx].Store(0)
					return f.interruptGroup.NewPacketConn(conn, interrupt.IsExternalConnectionFromContext(ctx)), nil
				}
			}
		}
		return nil, err
	}

	f.consecutiveFailures[idx].Store(0)
	return f.interruptGroup.NewPacketConn(conn, interrupt.IsExternalConnectionFromContext(ctx)), nil
}

// 切换到下一个可用节点
func (f *Failover) switchToNext(currentIdx int) {
	f.access.Lock()
	defer f.access.Unlock()

	fromNode := f.tags[currentIdx]

	for i := 1; i < len(f.outbounds); i++ {
		nextIdx := (currentIdx + i) % len(f.outbounds)

		// 跳过连续失败次数已达阈值的节点
		if int(f.consecutiveFailures[nextIdx].Load()) >= f.maxFailures {
			continue
		}

		toNode := f.tags[nextIdx]
		f.selected.Store(int32(nextIdx))
		f.logger.Warn("🔄 switched from ", fromNode, " to ", toNode)
		f.interruptGroup.Interrupt(f.interruptExternalConnections)

		// 🔥 通知 iOS 前端节点已切换
		platformInterface := service.FromContext[platform.Interface](f.ctx)
		if platformInterface != nil {
			f.logger.Warn("🔔 calling OnNodeSwitched callback: ", fromNode, " -> ", toNode)
			platformInterface.OnNodeSwitched(fromNode, toNode)
		} else {
			f.logger.Error("❌ platformInterface is nil, cannot notify iOS!")
		}
		return
	}

	// 所有节点都失败了，重置所有计数，回到第一个节点重试
	f.logger.Error("all outbounds failed, resetting and retry from primary")
	for i := range f.consecutiveFailures {
		f.consecutiveFailures[i].Store(0)
	}
	f.selected.Store(0)
	f.interruptGroup.Interrupt(f.interruptExternalConnections)

	// 🔥 通知 iOS 前端所有节点都失败了
	platformInterface := service.FromContext[platform.Interface](f.ctx)
	if platformInterface != nil {
		f.logger.Warn("🔔 calling OnAllNodesFailed callback")
		platformInterface.OnAllNodesFailed()
	} else {
		f.logger.Error("❌ platformInterface is nil, cannot notify iOS about all nodes failed!")
	}
}

// 主节点恢复检测循环
func (f *Failover) recoveryCheckLoop() {
	defer f.wg.Done() // 🔥 确保退出时通知 WaitGroup

	for {
		select {
		case <-f.close:
			f.logger.Debug("recovery check loop exiting (close signal)")
			return
		case <-f.ctx.Done():
			f.logger.Debug("recovery check loop exiting (context canceled)")
			return
		case <-f.recoveryTicker.C:
			// 🔥 检查 context 是否已取消
			if f.ctx.Err() != nil {
				return
			}
			f.checkPrimaryRecovery()
		}
	}
}

func (f *Failover) checkPrimaryRecovery() {
	// 🔥 检查 context 是否已取消
	if f.ctx.Err() != nil {
		return
	}

	currentIdx := int(f.selected.Load())
	if currentIdx == 0 {
		return // 已经在使用主节点
	}

	// 🔥 检查 outbounds 是否有效
	if len(f.outbounds) == 0 {
		return
	}

	primary := f.outbounds[0]

	// 🔥 使用更短的超时时间（3秒），避免长时间挂起
	var err error
	if f.recoveryURL != "" {
		ctx, cancel := context.WithTimeout(f.ctx, 3*time.Second)
		_, err = urltest.URLTest(ctx, f.recoveryURL, primary)
		cancel()
	} else {
		// 不配置 URL 时，使用 TCP 握手检测
		ctx, cancel := context.WithTimeout(f.ctx, 3*time.Second)
		conn, dialErr := primary.DialContext(ctx, "tcp", M.ParseSocksaddr("1.1.1.1:443"))
		cancel()
		if conn != nil {
			conn.Close()
		}
		err = dialErr
	}

	// 🔥 再次检查 context，避免在检测过程中被取消后继续操作
	if f.ctx.Err() != nil {
		return
	}

	if err == nil {
		f.access.Lock()
		f.selected.Store(0)
		f.consecutiveFailures[0].Store(0)
		f.access.Unlock()

		f.logger.Info("✅ primary outbound ", primary.Tag(), " recovered, switching back")
		f.interruptGroup.Interrupt(f.interruptExternalConnections)
	}
}

func (f *Failover) NewConnectionEx(ctx context.Context, conn net.Conn, metadata adapter.InboundContext, onClose N.CloseHandlerFunc) {
	ctx = interrupt.ContextWithIsExternalConnection(ctx)
	selected := f.getSelected()
	if selected == nil {
		N.CloseOnHandshakeFailure(conn, onClose, E.New("no available outbound"))
		return
	}
	if outboundHandler, isHandler := selected.(adapter.ConnectionHandlerEx); isHandler {
		outboundHandler.NewConnectionEx(ctx, conn, metadata, onClose)
	} else {
		f.connection.NewConnection(ctx, selected, conn, metadata, onClose)
	}
}

func (f *Failover) NewPacketConnectionEx(ctx context.Context, conn N.PacketConn, metadata adapter.InboundContext, onClose N.CloseHandlerFunc) {
	ctx = interrupt.ContextWithIsExternalConnection(ctx)
	selected := f.getSelected()
	if selected == nil {
		N.CloseOnHandshakeFailure(conn, onClose, E.New("no available outbound"))
		return
	}
	if outboundHandler, isHandler := selected.(adapter.PacketConnectionHandlerEx); isHandler {
		outboundHandler.NewPacketConnectionEx(ctx, conn, metadata, onClose)
	} else {
		f.connection.NewPacketConnection(ctx, selected, conn, metadata, onClose)
	}
}
