# thompsonsampling-orderrpc

汤普森采样的通用服务,用于从redis中获得目标物品的alpha,beta值,然后过beta分布随机出一个数值后做排序

## 状态保存

redis中的key模板为`Tompsonsampling::{业务命名空间}::{目标命名空间}::{候选物品}`,类型为`hash`,且hash中只有`alpha`和`beta`两个字段

如果找不到则默认为`0`

业务命名空间和目标命名空间默认为`__global__`

状态读写都使用原子操作因此不需要用分布式锁
