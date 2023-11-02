# EntCache

本项目灵感来源于 [ariga.io/entcache](https://github.com/ariga/entcache). 在数据缓存方面强化, 如果需要使用上下文缓存,请使用源项目.

在使用过程中,发现源项目在缓存方面有一些不足,比如:

- 无法影响数据变化. 比如在缓存中的数据,在数据库中被发生变更时,缓存中的数据无法获得通知.
- 缓存时间需要显示控制, 如某些查询需要长时间缓存,某些查询需要短时间缓存.

为了缓存该问题,项目结合了Ent的Hooks机制与模板功能的配合使用,针对中某些特定的查询,如Get方法以及Graphql中Noder查询.在约定的开发模式下,
尽量在发生数据变更后,UI能尽快的获取到最新的数据.

本项目并未实现在自行查询上的同步缓存更新,因此还需要使用者在开发过程中, 显式通过context的使用来合理的使用缓存.

## 设计

### 查询类型

在缓存KV中, 统一采用查询语句的Hash值做为键,来避免不同的库表命名冲空.但在查询方式中,我们区分了两种查询类型:

- Hash: 是开发者自定义的查询. 
- Key: 通过主键获取的数据的查询, 如Client.Get(context.Context, id any)同源的方法(如Only). 能主动影响数据变化.

对于这两种类型的查询,可以设定不同的缓存时间. 一般来说,Hash类型的查询,缓存时间会比较短,Key类型的查询,缓存时间会比较长.

### Key查询的淘汰

Key类型的缓存由发现变化到变化结束,其变化结束控制点在于Get触发执行,如果Get未执行时,则变化标记一直存在.

针对同源查询的缓存淘汰处理如下:

- 在标记期间,如果标记时间超过了上一次查询的缓存时间,则会触发缓存淘汰, 以更新最新的数据.
- 在标记期间,如果不存在Hash值(变更后的第一次查询),则会触发缓存淘汰,防止变化前的缓存.
- 未在标记期间的查询,如存在Hash值,则触发缓存淘汰.该Hash值的存储只在标记期间,存在说明存在旧数据.

### 内置缓存

内置的实现了Cache接口的TinyLFU缓存. 

## 使用

```
go get github.com/woocoos/entcache
```

在需要使用的Schema中,引入Hooks:

```go
func (User) Hooks() []ent.Hook {
	return []ent.Hook{
		entcache.DataChangeNotify(),
	}
}
```

我们可通过配置方式, 初始化Driver.
```yaml
entcache:
  # 可选, Hash类型查询的缓存时间,如果使用内置缓存,则为内置缓存的缓存时间.
  hashQueryTTL: 10s
  keyQueryTTL: 1h
  # 可选, 指定注册的缓存组件.
  cacheKey: entcache
  # 可选, 缓存前缀, 如果共用缓存组件则会有用.
  cachePrefix: "admin:"
```

```go
// cnf 为配置组件 
drv := entcache.NewDriver(entcache.Configuration(cnf.sub("entcache")))
client := ent.NewClient(ent.Driver(drv))
// 启用监听
go drv.Start(context.Background())
```

就可在代码中使用缓存了.
```go
ctx := entcache.WithEntryKey(context.Background(), "User", 1)
client.User.Get(ctx, 1)
```

### 利用模板简化 

在context的写法有些麻烦是不,并且还会传入无关的Key, 对变更也不友好.

我们内置对Ent Client模板的修改,在生成的代码中,让Get自动引入了缓存的上下文,可在`entc`使用如:

```go
func main() {
	// ....
	opts := []entc.Option{
		cachegen.QueryCache(),
	}
	if err := entc.Generate("./schema", &gen.Config{}, opts...); err != nil {
		log.Fatalf("running ent codegen: %v", err)
	}
}

```

Get方法就像平常那样使用了

如果你的项目已经存在模板的修改,那你已经知道怎么修改模板了,可以把调整模板的代码拷贝过来到你的模板中.

entgql也是同理修改,你可参考[TODO](integration/todo/ent/template/node.tmpl)中的修改.