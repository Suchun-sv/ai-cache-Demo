# DashScope文本向量+DashVector云原生向量数据库Demo本地与线上SAE环境搭建（线上评分58.5）

本文主要开源了我跑通的如标题所述的简单Demo，线上测评结果大概为58.5左右。涉及的点如目录所示，各位可以根据自己的进度来检查一下。因为我的go也是现学的，代码只能说能跑，期待和大佬们的交流。其次，对于刚接触的同学，最好能根据比赛要求搭建完SAE环境之后再开始实操。

全部的代码我开源在[github](https://github.com/Suchun-sv/ai-cache-Demo)上，如果觉得有用，欢迎大家star。

[TOC]

# 0. 背景

我们计划在 Higress 网关中开发一个 WebAssembly (Wasm) 插件，该插件将在 HTTP 请求的各个阶段（包括 requestHeader、requestBody、responseHeader、responseBody）进行介入，以便捕获相应的请求或响应数据，并执行定制的业务逻辑处理，实现对大型模型接口（如 OpenAI）的请求数据进行本地或云数据库缓存，并设计一个基于语义级别的缓存命中逻辑。通过这种方式，我们旨在降低响应时间并减少 Token 的消耗，从而提高系统效率并降低运营成本。

![图1: AI-Cache](DashScope%E6%96%87%E6%9C%AC%E5%90%91%E9%87%8F+DashVector%E4%BA%91%E5%8E%9F%E7%94%9F%E5%90%91%E9%87%8F%E6%95%B0%E6%8D%AE%E5%BA%93Demo%E6%9C%AC%E5%9C%B0%E4%B8%8E%E7%BA%BF%E4%B8%8ASAE%E7%8E%AF%E5%A2%83%E6%90%AD%E5%BB%BA%EF%BC%88%E7%BA%BF%20ab24d58c5a2046e6b84cd2187f320976/Untitled.png)

图1: AI-Cache

以图1为例，本比赛主要的问题可以归纳为：（1）如何根据Query字符串生成合适的Query向量⇒向量生成器选型。（2）如何根据Query向量进行语义级别的查找，快速找到合适的缓存向量⇒缓存命中逻辑设计。（3）如何管理大量的缓存⇒向量数据库选型及重复初始化逻辑。

# 1. Demo设计思路

简单的选型如下：

[向量生成器：阿里灵积通用文本向量接口](https://help.aliyun.com/zh/dashscope/developer-reference/text-embedding-quick-start?spm=a2c4g.11186623.0.0.5f8e4c5eVWPujd)

[向量数据库: 阿里向量检索服务DashVector]([https://help.aliyun.com/document_detail/2510225.html](https://help.aliyun.com/document_detail/2510225.html))

## 1.1 新建AI-cache插件并编写配置

首先，我们需要在相关服务平台进行注册并获取对应的访问令牌（token）。特别注意，Dashvector 的服务端点（endpoint）是针对每个用户唯一设定的，因此需要进行个别记录。可以按照以下简单格式来记录这些配置信息。针对插件名称的问题。可能会存在 "ai-cache"与现有插件冲突的问题，这可能导致一些错误。为避免这种情况，建议进行一些名称上的简单修改。

```jsx
dashvector:
  DashScopeKey: "YOUR_DASHSCOPE_KEY" // 这个是文本向量的key
  DashScopeServiceName: "qwen" // 重要，需要和scope对应的服务名匹配
  DashVectorCollection: "YOUR_CLUSTER_NAME"
  DashVectorEnd: "YOUR_VECTOR_ENDPOINT" 
  DashVectorKey: "YOUR_DASHVECTOR_KEY" // 这个是DASHVECTOR的key
  DashVectorServiceName: "DashVector" // 重要，需要新建一个vector对应的服务
  SessionID: "XXX" // 可用可不用，主要用于重复初始化逻辑
redis: // 重要
  serviceName: "redis.static"
  timeout: 2000
```

![Untitled](DashScope%E6%96%87%E6%9C%AC%E5%90%91%E9%87%8F+DashVector%E4%BA%91%E5%8E%9F%E7%94%9F%E5%90%91%E9%87%8F%E6%95%B0%E6%8D%AE%E5%BA%93Demo%E6%9C%AC%E5%9C%B0%E4%B8%8E%E7%BA%BF%E4%B8%8ASAE%E7%8E%AF%E5%A2%83%E6%90%AD%E5%BB%BA%EF%BC%88%E7%BA%BF%20ab24d58c5a2046e6b84cd2187f320976/Untitled%201.png)

## 1.2 配置服务

在进入到代码编写之前，需要在Higress的控制台新建服务和相应的配置路由，这里我默认使用的是开源版配置方案：

为什么要新建服务，是因为wasm无法使用协程调度，因此是无法简单的发起http请求的，需要通过封装的服务调用的方式来代替发起请求。

为什么要新建路由，默认的服务请求是http，所以需要新建路由，并打上https注解，才能正常发起https注解。

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

对于本地测试环境的搭建，采用[ **Higress + LobeChat 快速搭建私人GPT助理**](https://github.com/alibaba/higress/issues/1023)的方式启动两个docker容器进行本地测试，我配上了我的配置供各位参考：

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
      - DASHSCOPE_API_KEY= [YOUR KEY]
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

主要更改了higress的image，environment 以及 volumes的配置，启动和重启就是docker compose up -d，类似的不赘述了。

进一步地，我们需要了解本地代码编写的逻辑如何能反馈到测试环境中。

为了和线上测试环境一致，本文采用的还是本地编写代码⇒ 本地编译.wasm插件 ⇒ docker打包镜像并上传 ⇒ 修改本地测试环境配置中的镜像版本 ⇒ 开始测试并打印日志

具体的伪代码如下:

```bash
cd ${workspaceFolder}/higress/plugins/wasm-go
PLUGIN_NAME=ai-cache EXTRA_TAGS=proxy_wasm_version_0_2_100 make build 
修改版本号 (version.txt) // 这步可以自动化更新，也就是说每次编译自动更新版本号，代码太简单就不在这写了
export cur_version=$(cat ${workspaceFolder}/version.txt) && docker build -t [YOUR IMAGE BASE URL]:$cur_version -f Dockerfile . && docker push [YOUR IMAGE BASE URL]:$cur_version
// 这步很重要，可以修改本地测试环境配置中的镜像版本，就不需要每次再去控制台修改了
sudo bash -c \"sed -i 's|oci://registry.cn-hangzhou.aliyuncs.com/XXX:[0-9]*\\\\.[0-9]*\\\\.[0-9]*|oci://registry.cn-hangzhou.aliyuncs.com/XXX:$(cat version.txt)|g' data/wasmplugins/ai-cache-1.0.0.yaml\
```

如果你用vscode，建议把这些代码写在tasks.json里，当然写成脚本，每次直接启动也行。

# 3. 文本向量请求逻辑及缓存命中逻辑编写

本小节开始编写具体的业务逻辑代码，但是首先得明白现有的wasm不支持直接调用外部服务，也就是没法直接调现成的api sdk代码。只能使用封装好的接口来调用外部请求接口：[如何在wasm插件中请求外部服务](https://higress.io/zh-cn/docs/user/wasm-go/#%E5%9C%A8%E6%8F%92%E4%BB%B6%E4%B8%AD%E8%AF%B7%E6%B1%82%E5%A4%96%E9%83%A8%E6%9C%8D%E5%8A%A1)


全部的代码在[Demo](https://github.com/Suchun-sv/ai-cache-Demo)上，这里简单讲讲思路和核心代码。一个直观的思路是:

```md
1. query进来和redis中存的key匹配，若完全一致则直接返回
2. 否则请求text_embdding接口将query转换为query_embedding
3. 用quer_embedding和向量数据库中的向量做ANN search，返回最接近的key，并用阈值过滤
4. 若大于阈值，舍去，本轮cache未命中
5. 若小于阈值，则再次调用redis对新key做匹配
6. 在response阶段请求向量数据库新增query/query_emebdding
7. 在response阶段请求redis新增key/LLM返回结果
```

## 3.1 外部服务声明和注册

可以看到，难点主要在于如何实现1-5的连续外部服务调用。首先需要声明外部服务:

```bash
	DashVectorClient      wrapper.HttpClient `yaml:"-" json:"-"`
	DashScopeClient       wrapper.HttpClient `yaml:"-" json:"-"`
	redisClient    wrapper.RedisClient `yaml:"-" json:"-"`
```

并且在ParseConfig函数中注册外部服务：

```go
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

这里的ParseConfig函数是在http请求的各个阶段回调函数（requestHeader，requestBody，responseHeader，responseBody）之前的注册函数，但是似乎wasm插件会反复执行这个ParseConfig函数，并非在启动之后只执行一次？

## 3.2 连续callback实现连续服务调用

以onHttpRequestBody函数为例，代码需要写并发逻辑而非简单顺序逻辑。因而主体的函数代码是需要返回types.Action，也就是阻塞还是继续执行。我们目前的逻辑需要在处理完缓存命中逻辑之后才能继续执行操作，因此主体函数需要返回types.Pause，根据我们的处理逻辑调用的外部服务的回调函数中执行proxywasm.ResumeHttpRequest()或者直接返回proxywasm.SendHttpResponse，取决于是否命中。

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

需要注意的是，此缓存逻辑只能在返回 types.Action 类型的函数中使用，例如 `onHttpResponseBody`。由于`onHttpResponseBody`是一个流式处理函数，它不支持类似的缓存逻辑。这是因为在流式处理中，请求确实会被发送出去，但由于缺乏阻塞操作，无法触发回调函数。为解决这一问题，可以参考`wasm-go/pkg/wrapper/http_wrapper.go`中的实现，通过添加信号变量来进行必要的修改。

# 4. 线上环境代码更新

操作过程中，线上的测试环境也可以类似于本地环境的方式使用`sed`命令来处理配置。但请注意，在线上的环境（特指开源版本）中，所有配置修改需要通过进入webshell来完成。此外，若需查看日志，应先确保环境变量已正确配置，然后通过webshell使用` tail -f /var/log/higress/gateway.log`命令来实时跟踪日志文件。

```bash
sudo bash -c \"sed -i 's|oci://registry.cn-hangzhou.aliyuncs.com/XXX:[0-9]*\\\\.[0-9]*\\\\.[0-9]*|oci://registry.cn-hangzhou.aliyuncs.com/XXX:$(cat version.txt)|g' data/wasmplugins/ai-cache-1.0.0.yaml\
```

并且建议把log目录也挂载到NAS来做日志持久化（参考本地环境配置的yaml）。不过这不是SAE推荐的方法，可以考虑用阿里云的日志服务来做日志持久化替代（SLS）。

**最后再讨论如何在每次线上测试的时候保证数据库为空**。一种方法是每次都删除数据库后重建，这虽然可行，但存在初始化延迟以及可能忘记删除的风险。最开始，我尝试在 `ParseConfig`中初始化一个唯一的`session_uuid`作为标识符，然后在请求向量数据库和`Redis`时加入这个字段（关于如何使用字段进行过滤，Dashvector 文档中有相关 SQL 语法的介绍）。然而，由于`ParseConfig`函数并非只执行一次，我后来选择在更新配置的yaml文件时手动插入此标识符。

**最后恭祝大家能取得好成绩，有问题可以在群里@我（苏淳sv），或者在github上发issue。**