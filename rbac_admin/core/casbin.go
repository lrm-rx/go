package core

import (
	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	"github.com/sirupsen/logrus"
	"rbac.admin/global"
)

func InitCasbin() *casbin.CachedEnforcer {
	a, _ := gormadapter.NewAdapterByDB(global.DB)
	casbinModel := `[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub) && r.obj == p.obj && r.act == p.act`
	m, err := model.NewModelFromString(casbinModel)
	if err != nil {
		logrus.Error("casbin模型失败!", err)
	}
	e, _ := casbin.NewCachedEnforcer(m, a)
	e.SetExpireTime(60 * 60)
	_ = e.LoadPolicy()
	return e
}
