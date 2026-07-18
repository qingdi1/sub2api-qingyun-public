package middleware

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// AuthSubject is the minimal authenticated identity stored in gin context.
// Decision: {UserID int64, Concurrency int}
type AuthSubject struct {
	UserID      int64
	Concurrency int
}

func GetAuthSubjectFromContext(c *gin.Context) (AuthSubject, bool) {
	value, exists := c.Get(string(ContextKeyUser))
	if !exists {
		return AuthSubject{}, false
	}
	subject, ok := value.(AuthSubject)
	return subject, ok
}

func GetUserRoleFromContext(c *gin.Context) (string, bool) {
	value, exists := c.Get(string(ContextKeyUserRole))
	if !exists {
		return "", false
	}
	role, ok := value.(string)
	return role, ok
}

// SetDemoUserInContext records the virtual demo identity after a JWT has been
// validated. No repository lookup should occur for contexts containing it.
func SetDemoUserInContext(c *gin.Context, user *service.User) {
	if c == nil || user == nil || !user.IsDemo {
		return
	}
	c.Set(string(ContextKeyDemoUser), user)
}

func GetDemoUserFromContext(c *gin.Context) (*service.User, bool) {
	if c == nil {
		return nil, false
	}
	value, exists := c.Get(string(ContextKeyDemoUser))
	if !exists {
		return nil, false
	}
	user, ok := value.(*service.User)
	return user, ok && user != nil && user.IsDemo
}

func IsDemoRequest(c *gin.Context) bool {
	_, ok := GetDemoUserFromContext(c)
	return ok
}
