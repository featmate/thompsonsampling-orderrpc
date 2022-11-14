# thompsonsampling-orderrpc

汤普森采样的通用服务,用于从redis中获得目标物品的alpha,beta值,然后过beta分布随机出一个数值后做排序

## 使用

这个服务至少需要连接一个redis用于水平扩展时作为共享状态的保存位置.请不要直接使用连接的redis来修改参数状态.如果需要保存和管理范围内的候选人状态可以通过设置启动`ScopeObserveMode`并连接以个etcd来实现.

### 范围(scope)

范围限定使用字段使用`business`和`targe`设置范围,如果不设置则使用启动设置中的对应值作为默认值.范围的作用是限定一个命名空间,很多时候业务上需要通过范围隔离不同业务的数据.默认并不保存范围的信息,如果要启动这一特性可以通过设置启动`ScopeObserveMode`并连接以个etcd来实现.

### 状态保存

要修改状态请调用`UpdateCandidate`接口.注意更新请求中的ttl为key的过期时间,默认值由启动参数`DefaultKeyTTL`来控制,它是防止redis的key忘记删除而设置的,在每次update后都会刷新.

`UpdateCandidate`接口会维护一个metakey用于存放每个业务的每个目标其下面更新过的候选人(不会因为过期而删除其中的内容,但这个key本生会过期,遵从ttl设置)

`UpdateCandidate`接口有两种模式

1. 重置模式, 将key的值重置为指定的值,注意也会重置meta中的候选集
2. 增量模式, 将key的值与提供的增量叠加

建议更多的使用重置模式,以确保事件的时效性.具体就是按固定的周期或者总量批量的的重置参数.

### 操作

支持排序和top操作,都是将候选集填入去Redis中查找到alpha和beta值进行随机,将结果排序后给出对应的结果.本服务并不保存业务的候选集信息,候选集是由请求中带入的.同时也更加建议在请求时不要带入过多候选,可以在其前面加上一层召回先筛选出基本符合要求的候选集再计算汤普森采样,以避免占用太多响应时间.