# DashScope文本向量+DashVector云原生向量数据库Demo本地与线上SAE环境搭建（线上评分58.5）

本文旨在分享一个简单的示例，具体内容如标题所述，我已成功实现并开源。文章的内容结构将按目录展开，读者可以根据自身进度逐一检查。我尚是 Go 语言的初学者，因此代码功能性为主，尚有优化空间。我非常期待与各位专家的进一步交流。对于初学者，我建议在比赛要求的基础上先搭建完整的 SAE 环境，然后再着手实操。

全部的代码我开源在 [github](https://github.com/Suchun-sv/ai-cache-Demo) 上，如果觉得有用，欢迎大家star。

[TOC]

# 0. 背景

我们要在 Higress 网关中编写 WebAssembly（wasm）插件，使得在 http 请求的各个阶段（requestHeader，requestBody，responseHeader，responseBody）能够将相应的请求或返回捕获进行业务逻辑的处理。具体到本比赛，主要需要实现的是缓存对大模型的请求（openai接口的形式）在本地（或云数据库），并设计语义级别的缓存命中逻辑来实现降低响应请求且减少 token 费用的目的。

![图1: AI-Cache](DashScope%E6%96%87%E6%9C%AC%E5%90%91%E9%87%8F+DashVector%E4%BA%91%E5%8E%9F%E7%94%9F%E5%90%91%E9%87%8F%E6%95%B0%E6%8D%AE%E5%BA%93Demo%E6%9C%AC%E5%9C%B0%E4%B8%8E%E7%BA%BF%E4%B8%8ASAE%E7%8E%AF%E5%A2%83%E6%90%AD%E5%BB%BA%EF%BC%88%E7%BA%BF%20ab24d58c5a2046e6b84cd2187f320976/Untitled.png)

图1: AI-Cache

以图1为例，本比赛主要的问题可以归纳为：（1）如何根据 Query 字符串生成合适的 Query 向量⇒向量生成器选型。（2）如何根据 Query 向量进行语义级别的查找，快速找到合适的缓存向量⇒缓存命中逻辑设计。（3）如何管理大量的缓存⇒向量数据库选型及重复初始化逻辑。

# 1. Demo设计思路

简单的选型如下：

[向量生成器：阿里灵积通用文本向量接口](https://help.aliyun.com/zh/dashscope/developer-reference/text-embedding-quick-start?spm=a2c4g.11186623.0.0.5f8e4c5eVWPujd)

[向量数据库: 阿里向量检索服务DashVector](https://cn.aliyun.com/product/ai/DashVector)



## 1.1 新建AI-cache插件并编写配置

首先简单的在各家注册并获取到 token，注意 DashVector 的 endpoint 是每个用户唯一的，所以需要单独记录。可以简单的把配置记录如下。另外注意最新版本插件名若为 "ai-cache" 可能会报错？可以做些简单修改

```bash
Dash:
  dashScopeKey: "YOUR DASHSCOPE KEY" // 这个是文本向量的key
  dashScopeServiceName: "qwen" // 重要，需要和scope对应的服务名匹配
  dashVectorCollection: "YOUR CLUSTER NAME"
  dashVectorEnd: "YOUR VECTOR END" 
  dashVectorKey: "YOUR DASHVECTOR KEY" // 这个是DASHVECTOR的key
  dashVectorServiceName: "DashVector" // 重要，需要新建一个vector对应的服务
  sessionID: "XXX" // 可用可不用，主要用于重复初始化逻辑
redis: // 重要
  serviceName: "redis.static"
  timeout: 2000
```

![Untitled](DashScope%E6%96%87%E6%9C%AC%E5%90%91%E9%87%8F+DashVector%E4%BA%91%E5%8E%9F%E7%94%9F%E5%90%91%E9%87%8F%E6%95%B0%E6%8D%AE%E5%BA%93Demo%E6%9C%AC%E5%9C%B0%E4%B8%8E%E7%BA%BF%E4%B8%8ASAE%E7%8E%AF%E5%A2%83%E6%90%AD%E5%BB%BA%EF%BC%88%E7%BA%BF%20ab24d58c5a2046e6b84cd2187f320976/Untitled%201.png)

## 1.2 配置服务

在进入到代码编写之前，需要在 Higress 的控制台新建服务和相应的配置路由，这里我默认使用的是开源版配置方案：

为什么要新建服务？是因为 wasm 无法使用协程调度，因此是无法简单的发起 http 请求的，需要通过封装的服务调用的方式来代替发起请求。

为什么要新建路由？默认的服务请求是 http，所以需要新建路由，并打上 https 注解，才能正常发起 https 请求。

![Untitled](DashScope%E6%96%87%E6%9C%AC%E5%90%91%E9%87%8F+DashVector%E4%BA%91%E5%8E%9F%E7%94%9F%E5%90%91%E9%87%8F%E6%95%B0%E6%8D%AE%E5%BA%93Demo%E6%9C%AC%E5%9C%B0%E4%B8%8E%E7%BA%BF%E4%B8%8ASAE%E7%8E%AF%E5%A2%83%E6%90%AD%E5%BB%BA%EF%BC%88%E7%BA%BF%20ab24d58c5a2046e6b84cd2187f320976/Untitled%202.png)

![Untitled](DashScope%E6%96%87%E6%9C%AC%E5%90%91%E9%87%8F+DashVector%E4%BA%91%E5%8E%9F%E7%94%9F%E5%90%91%E9%87%8F%E6%95%B0%E6%8D%AE%E5%BA%93Demo%E6%9C%AC%E5%9C%B0%E4%B8%8E%E7%BA%BF%E4%B8%8ASAE%E7%8E%AF%E5%A2%83%E6%90%AD%E5%BB%BA%EF%BC%88%E7%BA%BF%20ab24d58c5a2046e6b84cd2187f320976/Untitled%203.png)

![Untitled](DashScope%E6%96%87%E6%9C%AC%E5%90%91%E9%87%8F+DashVector%E4%BA%91%E5%8E%9F%E7%94%9F%E5%90%91%E9%87%8F%E6%95%B0%E6%8D%AE%E5%BA%93Demo%E6%9C%AC%E5%9C%B0%E4%B8%8E%E7%BA%BF%E4%B8%8ASAE%E7%8E%AF%E5%A2%83%E6%90%AD%E5%BB%BA%EF%BC%88%E7%BA%BF%20ab24d58c5a2046e6b84cd2187f320976/Untitled%204.png)

![Untitled](DashScope%E6%96%87%E6%9C%AC%E5%90%91%E9%87%8F+DashVector%E4%BA%91%E5%8E%9F%E7%94%9F%E5%90%91%E9%87%8F%E6%95%B0%E6%8D%AE%E5%BA%93Demo%E6%9C%AC%E5%9C%B0%E4%B8%8E%E7%BA%BF%E4%B8%8ASAE%E7%8E%AF%E5%A2%83%E6%90%AD%E5%BB%BA%EF%BC%88%E7%BA%BF%20ab24d58c5a2046e6b84cd2187f320976/Untitled%205.png)

```bash
higress.io/backend-protocol: https
higress.io/proxy-ssl-name: YOUR_ENDPOINT
higress.io/proxy-ssl-server-name: on
```

# 2. 本地测试环境搭建和代码更新逻辑

采用[ **Higress + LobeChat 快速搭建私人GPT助理**](https://github.com/alibaba/higress/issues/1023)的方式起两个Docker容器进行本地测试，我配上了我的配置供各位参考：

```bash
version: '3.9'

networks:
  higress-net:
    external: false

services:
  higress:
    image: registry.cn-hangzhou.aliyuncs.com/ztygw/aio-redis:1.4.0-rc.1
    environment:
      - GATEWAY_COMPONENT_LOG_LEVEL=misc:error,wasm:debug // 重要，开启日志
      - CONFIG_TEMPLATE=ai-proxy
      - DEFAULT_AI_SERVICE=qwen
      - DASHSCOPE_API_KEY= [YOUR_KEY]
    networks:
      - higress-net
    ports:
      - "9080:8080/tcp"
      - "9001:8001/tcp"
    volumes:
      - 本地data目录:/data
      - 本地log目录:/var/log/higress/ //重要，方便你在restrat之后查看目录
    restart: always
  lobechat:
    image: lobehub/lobe-chat
    environment:
      - CODE=123456ed
      - OPENAI_API_KEY=unused
      - OPENAI_PROXY_URL=http://higress:8080/v1
    networks:
      - higress-net
    ports:
      - "3210:3210/tcp"
    restart: always

```

主要更改了 Higress 的 image，environment 以及 volumes 的配置，启动和重启就是 `Docker compose up -d`，类似的不赘述了。

进一步地，我们需要了解本地代码编写的逻辑如何能反馈到测试环境中。

为了和线上测试环境一致，本文采用的还是本地编写代码⇒ 本地编译 wasm 插件 ⇒  Docker 打包镜像并上传 ⇒ 修改本地测试环境配置中的镜像版本 ⇒ 开始测试并打印日志

具体的伪代码如下:

```bash
cd ${workspaceFolder}/higress/plugins/wasm-go
PLUGIN_NAME=ai-cache EXTRA_TAGS=proxy_wasm_version_0_2_100 make build 
修改版本号（version.txt）// 这步可以自动化更新，也就是说每次编译自动更新版本号，代码太简单就不在这写了
export cur_version=$(cat ${workspaceFolder}/version.txt) && docker build -t [YOUR IMAGE BASE URL]:$cur_version -f Dockerfile . && docker push [YOUR IMAGE BASE URL]:$cur_version
// 这步很重要，可以修改本地测试环境配置中的镜像版本，就不需要每次再去控制台修改了
sudo bash -c \"sed -i 's|oci://registry.cn-hangzhou.aliyuncs.com/XXX:[0-9]*\\\\.[0-9]*\\\\.[0-9]*|oci://registry.cn-hangzhou.aliyuncs.com/XXX:$(cat version.txt)|g' data/wasmplugins/ai-cache-1.0.0.yaml\
```

如果你用 VSCode，建议把这些代码写在 tasks.json 里。当然写成脚本，然后每次直接启动也行。

# 3. 文本向量请求逻辑及缓存命中逻辑编写

本小节开始写具体的代码了，但是首先需要了解现有的 wasm 不支持直接调用外部服务，也就是没法直接调现成的 api sdk 代码。只能走封装好的调用外部请求接口：[如何在插件中请求外部服务](https://higress.io/zh-cn/docs/user/wasm-go/#%E5%9C%A8%E6%8F%92%E4%BB%B6%E4%B8%AD%E8%AF%B7%E6%B1%82%E5%A4%96%E9%83%A8%E6%9C%8D%E5%8A%A1)。

全部的代码在 [github](https://github.com/Suchun-sv/ai-cache-Demo?spm=a2c22.21852664.0.0.45972df20aJlUt) 上，这里简单讲讲思路和核心代码。首先，本文采用的直观的思路是:

```md
1. query进来和redis中存的key匹配，若完全一致则直接返回
2. 否则请求text_embdding接口将query转换为query_embedding
3. 用quer_embedding和向量数据库中的向量做ANN search，返回最接近的key，并用阈值过滤
4. 若大于阈值，舍去，本轮cache未命中
5. 若小于阈值，则再次调用redis对新key做匹配。
6. 在response阶段请求向量数据库新增query/query_emebdding
7. 在response阶段请求redis新增key/LLM返回结果
```

## 3.1 外部服务声明和注册

可以看到，难点主要在于如何实现 1-5 的连续外部服务调用。在 Higress 相关的配置上，我们首先需要声明外部服务:

```bash
DashVectorClient      wrapper.HttpClient `yaml:"-" json:"-"`
DashScopeClient       wrapper.HttpClient `yaml:"-" json:"-"`
redisClient    wrapper.RedisClient `yaml:"-" json:"-"`
```

并且在 `ParseConfig` 函数中注册外部服务：
```bash
c.DashVectorInfo.DashVectorClient = wrapper.NewClusterClient(wrapper.DnsCluster{
	ServiceName: c.DashVectorInfo.DashVectorServiceName,
	Port:        443,
	Domain:      c.DashVectorInfo.DashVectorAuthApiEnd,
})
c.DashVectorInfo.DashScopeClient = wrapper.NewClusterClient(wrapper.DnsCluster{
	ServiceName: c.DashVectorInfo.DashScopeServiceName,
	Port:        443,
	Domain:      "dashscope.aliyuncs.com",
})
```

这里的 `ParseConfig` 函数是在 http 请求的各个阶段回调函数（requestHeader，requestBody，responseHeader，responseBody）之前的注册函数，但是似乎 wasm 插件会反复执行这个 ParseConfig 函数，并非在启动之后只执行一次？

## 3.2 连续callback实现连续服务调用

以 `onHttpRequestBody` 函数为例，代码需要写并发逻辑而非简单顺序逻辑。因而主体的函数代码是需要返回 `types.Action`，也就是声明阻塞还是继续执行。我们目前的逻辑需要在处理完缓存命中逻辑之后才能继续执行操作，因此主体函数需要返回 `types.pause`, 根据我们的处理逻辑调用的外部服务的回调函数中执行`proxywasm.ResumeHttpRequest()`或者直接返回`proxywasm.SendHttpResponse()`，取决于是否命中。

```go
err := config.redisClient.Get(config.CacheKeyPrefix+key, func(response resp.Value) {
	if err := response.Error(); err != nil || response.IsNull() {
		if err != nil {
			log.Warnf("redis get key:%s failed, err:%v", key, err)
		}
		if response.IsNull() {
			log.Warnf("cache miss, key:%s", key)
		}
		// 回调函数1
		config.DashVectorInfo.DashScopeClient.Post(
			Emb_url,
			Emb_headers,
			Emb_requestBody,
			func(statusCode int, responseHeaders http.Header, responseBody []byte) {
				log.Infof("statusCode:%d, responseBody:%s", statusCode, string(responseBody))
				if statusCode != 200 {
					log.Errorf("Failed to fetch embeddings, statusCode: %d, responseBody: %s", statusCode, string(responseBody))
					// result = nil
					ctx.SetContext(QueryEmbeddingKey, nil)
				} else {
					// 回调函数2
					config.redisClient.Set(config.CacheKeyPrefix+key, Text_embedding, func(response resp.Value) {
						if err := response.Error(); err != nil {
							log.Warnf("redis set key:%s failed, err:%v", key, err)
							proxywasm.ResumeHttpRequest() // 未命中，取消暂停，继续请求
							return
						}
						log.Infof("Successfully set key:%s", key)
						// 确认存了之后继续和database交互
						vector_url, vector_request, vector_headers, err := PerformQuery(config, Text_embedding)
						if err != nil {
							log.Errorf("Failed to perform query, err: %v", err)
							proxywasm.ResumeHttpRequest() // 未命中，取消暂停，继续请求
							return
						}
						// config.DashVectorInfo.DashVectorClient.Post(
						config.DashVectorInfo.DashVectorClient.Post(
							vector_url,
							vector_headers,
							vector_request,
							...
										// 回调函数三
								if !stream {
									proxywasm.SendHttpResponse(200, [][2]string{{"content-type", "application/json; charset=utf-8"}}, []byte(fmt.Sprintf(config.ReturnResponseTemplate, most_similar_key)), -1)
								} else {
									proxywasm.SendHttpResponse(200, [][2]string{{"content-type", "text/event-stream; charset=utf-8"}}, []byte(fmt.Sprintf(config.ReturnStreamResponseTemplate, most_similar_key)), -1)
								} // 命中了，直接返回命中key对应的value
								proxywasm.ResumeHttpRequest()
							},
							100000)
					})
				}
			},
			10000)
	} else {
		log.Warnf("cache hit, key:%s", key)
		ctx.SetContext(CacheKeyContextKey, nil)
		// 命中了，直接返回命中key对应的value
		if !stream {
			proxywasm.SendHttpResponse(200, [][2]string{{"content-type", "application/json; charset=utf-8"}}, []byte(fmt.Sprintf(config.ReturnResponseTemplate, response.String())), -1)
		} else {
			proxywasm.SendHttpResponse(200, [][2]string{{"content-type", "text/event-stream; charset=utf-8"}}, []byte(fmt.Sprintf(config.ReturnStreamResponseTemplate, response.String())), -1)
		}
	}
})
if err != nil {
	log.Error("redis access failed")
	return types.ActionContinue
}
return types.ActionPause
```

另外，该逻辑只有在返回值为 `types.Action` 的函数中才能用，如`onHttpResponseBody` 这个流式处理的函数是没法类似地处理，只能保证请求发出去，但是因为没有阻塞操作，无法调用callback函数。如有需求，可以参照`wasm-go/pkg/wrapper/http_wrapper.go`加上信号变量修改。

# 4. 线上环境代码更新

和本地环境类似，线上的环境也可以直接采用 `sed` 命令来更新 wasm 插件，但是需要注意的是，线上的环境配置（单指开源版）需要进入到 webshell 里面进行，查看log 也需要在配置好环境变量后进 webshell 执行 `tail -f /var/log/higress/gateway.log` 命令。

```bash
sudo bash -c \"sed -i 's|oci://registry.cn-hangzhou.aliyuncs.com/XXX:[0-9]*\\\\.[0-9]*\\\\.[0-9]*|oci://registry.cn-hangzhou.aliyuncs.com/XXX:$(cat version.txt)|g' data/wasmplugins/ai-cache-1.0.0.yaml\
```

并且建议把 log 目录也挂载到 NAS 里来做日志持久化（参考本地环境配置的 yaml ）。不过这并不是 SAE 推荐的方法，可能会有潜在的 NAS 并发问题。如有需求，可以考虑采用 SLS 日志服务来做日志持久化。

**最后如何保证每次的环境里的数据库都是处于初始化状态?** 一种方法是每次都删除数据库后重建，这虽然可行，但存在初始化延迟以及可能忘记删除的风险。我最开始尝试在 `ParseConfig` 中初始化一个唯一的 `session_uuid` 作为标识符，然后在请求向量数据库和 Redis 时加入这个字段。关于如何使用字段进行过滤（ Dashvector 文档中有相关 SQL 语法的介绍）。然而，由于 `ParseConfig` 函数并非只执行一次，我后来选择在更新配置时手动插入此标识符。为此，我编写了一个本地脚本来生成所需的 `sed` 命令，以自动化这一过程。

**最后恭祝大家能取得好成绩，有问题可以在群里@我（苏淳sv），或者在github上发issue。**