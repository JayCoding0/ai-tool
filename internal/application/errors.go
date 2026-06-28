// Package application 应用服务层
package application

import "errors"

// ErrForbidden 表示当前用户无权访问目标资源（越权）。
// HTTP 接口层应将其映射为 403 Forbidden。
var ErrForbidden = errors.New("无权访问该资源")

// ErrUnauthorized 表示请求未携带有效身份（未登录）。
// HTTP 接口层应将其映射为 401 Unauthorized。
var ErrUnauthorized = errors.New("请先登录")
