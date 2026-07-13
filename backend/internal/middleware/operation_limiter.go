package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// OperationPolicy 描述单个高成本操作的进程内准入控制策略。
//
// 注意：策略值由 router 按操作类别显式指定，属于安全评审决策；修改需同步评审。
type OperationPolicy struct {
	// MaxConcurrent 是该操作允许的最大并发请求数；<=0 表示不限制并发，仅做速率控制。
	// 达到上限时新请求在请求上下文内排队等待槽位释放，仅在客户端断开/超时时返回 429，
	// 以避免对批量封面加载等正常突发请求造成瞬时失败。
	MaxConcurrent int
	// MaxRequests 是 Window 窗口内允许的最大请求次数；<=0 表示不做速率控制。
	MaxRequests int
	// Window 是速率统计的滑动窗口时长。
	Window time.Duration
}

const (
	operationRateLimitedCode = "OPERATION_RATE_LIMITED"
	operationBusyCode        = "OPERATION_BUSY"
)

type operationState struct {
	policy OperationPolicy
	sem    chan struct{}

	winMu  sync.Mutex
	now    func() time.Time
	stamps []time.Time
}

// OperationLimiter 为多类高成本操作提供进程内准入控制。
// 不信任客户端身份头，通过稳定的 429 错误让前端区分过快请求与并发已满。
type OperationLimiter struct {
	mu      sync.Mutex
	perOp   map[string]*operationState
	nowFunc func() time.Time
}

// NewOperationLimiter 创建空的准入控制器。
func NewOperationLimiter() *OperationLimiter {
	return &OperationLimiter{
		perOp:   make(map[string]*operationState),
		nowFunc: time.Now,
	}
}

func (l *OperationLimiter) stateFor(op string, policy OperationPolicy) *operationState {
	l.mu.Lock()
	defer l.mu.Unlock()
	if state, ok := l.perOp[op]; ok {
		return state
	}
	state := &operationState{policy: policy, now: l.nowFunc}
	if policy.MaxConcurrent > 0 {
		state.sem = make(chan struct{}, policy.MaxConcurrent)
	}
	l.perOp[op] = state
	return state
}

// Middleware 返回按 op 与 policy 进行准入控制的 gin 中间件。
func (l *OperationLimiter) Middleware(op string, policy OperationPolicy) gin.HandlerFunc {
	state := l.stateFor(op, policy)
	return func(c *gin.Context) {
		// 速率：滑动窗口内请求数超限时直接 429（过快请求）。
		if !state.allowRate() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"success": false,
				"error": gin.H{
					"code":    operationRateLimitedCode,
					"message": "操作请求过于频繁，请稍后再试",
				},
			})
			return
		}

		// 并发：在请求上下文内排队等待空闲槽位；仅当客户端断开/超时（上下文取消）时返回 429。
		if state.sem != nil {
			select {
			case state.sem <- struct{}{}:
				defer func() { <-state.sem }()
			case <-c.Request.Context().Done():
				c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
					"success": false,
					"error": gin.H{
						"code":    operationBusyCode,
						"message": "操作并发已满，请稍后再试",
					},
				})
				return
			}
		}

		c.Next()
	}
}

// allowRate 在滑动窗口内统计请求次数，超限返回 false。
func (s *operationState) allowRate() bool {
	if s.policy.MaxRequests <= 0 || s.policy.Window <= 0 {
		return true
	}
	now := s.now()
	cutoff := now.Add(-s.policy.Window)

	s.winMu.Lock()
	defer s.winMu.Unlock()

	// 丢弃窗口外的旧记录。
	i := 0
	for ; i < len(s.stamps); i++ {
		if s.stamps[i].After(cutoff) {
			break
		}
	}
	s.stamps = s.stamps[i:]

	if len(s.stamps) >= s.policy.MaxRequests {
		return false
	}
	s.stamps = append(s.stamps, now)
	return true
}
