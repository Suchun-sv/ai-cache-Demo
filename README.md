# DashScope文本向量+DashVector云原生向量数据库Demo本地与线上SAE环境搭建（线上评分58.5）

本文主要开源了我跑通的如标题所述的简单Demo，线上测评结果大概为58.5左右。涉及的点如目录所示，各位可以根据自己的进度来检查一下。因为我的go也是现学的，代码只能说能跑，期待和大佬们的交流。其次，对于刚接触的同学，最好能根据比赛要求搭建完SAE环境之后再开始实操。

全部的代码我开源在[github](https://github.com/Suchun-sv/ai-cache-Demo)上，如果觉得有用，欢迎大家star。

[TOC]

# 0. 背景

我们要在Higress网关中编写 WebAssembly(wasm)插件，使得在http请求的各个阶段（requestHeader，requestBody，responseHeader，responseBody)能够将相应的请求或返回捕获进行业务逻辑的处理。具体到本比赛，主要需要实现的是缓存对大模型的请求（openai接口的形式）在本地（或云数据库），并设计语义级别的缓存命中逻辑来实现降低响应请求且减少token费用的目的。

![图1: AI-Cache](DashScope%E6%96%87%E6%9C%AC%E5%90%91%E9%87%8F+DashVector%E4%BA%91%E5%8E%9F%E7%94%9F%E5%90%91%E9%87%8F%E6%95%B0%E6%8D%AE%E5%BA%93Demo%E6%9C%AC%E5%9C%B0%E4%B8%8E%E7%BA%BF%E4%B8%8ASAE%E7%8E%AF%E5%A2%83%E6%90%AD%E5%BB%BA%EF%BC%88%E7%BA%BF%20ab24d58c5a2046e6b84cd2187f320976/Untitled.png)

图1: AI-Cache

以图1为例，本比赛主要的问题可以归纳为：（1）如何根据Query字符串生成合适的Query向量⇒向量生成器选型。（2）如何根据Query向量进行语义级别的查找，快速找到合适的缓存向量⇒缓存命中逻辑设计。（3）如何管理大量的缓存⇒向量数据库选型及重复初始化逻辑。

# 1. Demo设计思路

简单的选型如下：

[向量生成器：阿里灵积通用文本向量接口](https://help.aliyun.com/zh/dashscope/developer-reference/text-embedding-quick-start?spm=a2c4g.11186623.0.0.5f8e4c5eVWPujd)

[向量数据库: 阿里向量检索服务DashVector]([https://help.aliyun.com/document_detail/2510225.html](https://help.aliyun.com/document_detail/2510225.html))

## 1.1 新建AI-cache插件并编写配置

首先简单的在各家注册并获取到token，注意dashvector的endpoint是每个用户唯一的，所以需要单独记录。可以简单的把配置记录如下，注意最新版本插件名若为ai-cache可能会报错？可以做些简单修改

```jsx
dashvector:
  DashScopeKey: "YOUR DASHSCOPE KEY" // 这个是文本向量的key
  DashScopeServiceName: "qwen" // 重要，需要和scope对应的服务名匹配
  DashVectorCollection: "YOUR CLUSTER NAME"
  DashVectorEnd: "YOUR VECTOR END" 
  DashVectorKey: "YOUR DASHVECTOR KEY" // 这个是DASHVECTOR的key
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

```jsx
higress.io/backend-protocol: https
higress.io/proxy-ssl-name: YOUR END POINT
higress.io/proxy-ssl-server-name: on
```

# 2. 本地测试环境搭建和代码更新逻辑

采用[ **Higress + LobeChat 快速搭建私人GPT助理**](https://github.com/alibaba/higress/issues/1023)的方式起两个docker容器进行本地测试，我配上了我的配置供各位参考：

```jsx
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

```jsx
cd ${workspaceFolder}/higress/plugins/wasm-go
PLUGIN_NAME=ai-cache EXTRA_TAGS=proxy_wasm_version_0_2_100 make build 
修改版本号 (version.txt) // 这步可以自动化更新，也就是说每次编译自动更新版本号，代码太简单就不在这写了
export cur_version=$(cat ${workspaceFolder}/version.txt) && docker build -t [YOUR IMAGE BASE URL]:$cur_version -f Dockerfile . && docker push [YOUR IMAGE BASE URL]:$cur_version
// 这步很重要，可以修改本地测试环境配置中的镜像版本，就不需要每次再去控制台修改了
sudo bash -c \"sed -i 's|oci://registry.cn-hangzhou.aliyuncs.com/XXX:[0-9]*\\\\.[0-9]*\\\\.[0-9]*|oci://registry.cn-hangzhou.aliyuncs.com/XXX:$(cat version.txt)|g' data/wasmplugins/ai-cache-1.0.0.yaml\
```

如果你用vscode，建议把这些代码写在tasks.json里，当然写成脚本，每次直接启动也行。

# 3. 文本向量请求逻辑及缓存命中逻辑编写

本小节终于开始写具体的代码了，但是首先得明白现有的wasm不支持直接调用外部服务，也就是没法直接调现成的api sdk代码。只能走封装好的调用外部请求接口：

[https://higress.io/zh-cn/docs/user/wasm-go/#在插件中请求外部服务](https://higress.io/zh-cn/docs/user/wasm-go/#%E5%9C%A8%E6%8F%92%E4%BB%B6%E4%B8%AD%E8%AF%B7%E6%B1%82%E5%A4%96%E9%83%A8%E6%9C%8D%E5%8A%A1)

全部的代码在https://github.com/Suchun-sv/ai-cache-Demo上，这里简单讲讲思路和核心代码。一个直观的思路是:

```jsx
1.query进来和redis中存的key匹配，若完全一致则直接返回
2.否则请求text_embdding接口将query转换为query_embedding
3.用quer_embedding和向量数据库中的向量做ANN search，返回最接近的key，并用阈值过滤
4.若大于阈值，舍去，本轮cache未命中
5.若小于阈值，则再次调用redis对新key做匹配。
6.在response阶段请求向量数据库新增query/query_emebdding
7.在response阶段请求redis新增key/LLM返回结果
```

## 3.1 外部服务声明和注册

可以看到，难点主要在于如何实现1-5的连续外部服务调用。首先需要声明外部服务:

```jsx
	DashVectorClient      wrapper.HttpClient `yaml:"-" json:"-"`
	DashScopeClient       wrapper.HttpClient `yaml:"-" json:"-"`
	redisClient    wrapper.RedisClient `yaml:"-" json:"-"`
```

并且在ParseConfig函数中注册外部服务：

```jsx
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

这里的ParseConfig函数是在http请求的各个阶段回调函数（requestHeader，requestBody，responseHeader，responseBody)之前的注册函数，但是似乎wasm插件会反复执行这个ParseConfig函数，并非在启动之后只执行一次？

## 3.2 连续callback实现连续服务调用

以onHttpRequestBody函数为例，代码需要写并发逻辑而非简单顺序逻辑，（我上次写还是本科计网，哈哈）。因而主体的函数代码是需要返回types.Action，也就是阻塞还是继续执行。我们目前的逻辑需要在处理完缓存命中逻辑之后才能继续执行操作，因此主体函数需要返回types.Pause, 根据我们的处理逻辑调用的外部服务的回调函数中执行proxywasm.ResumeHttpRequest()或者直接返回proxywasm.SendHttpResponse，取决于是否命中。

```jsx
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

另外，该逻辑只有在返回值为types.Action的函数中才能用，如onHttpResponseBody这个流式处理的函数是没法这么整的，只能保证请求发出去，但是因为没有阻塞操作，无法调用callback函数。如果代码能力强，可以参照`wasm-go/pkg/wrapper/http_wrapper.go`加上信号变量修改。（我就算了）

# 4. 线上环境代码更新

和本地环境类似，直接采用sed命令，但是需要注意的是，线上的环境配置(单指开源版）需要进入到webshell里面进行，看log也需要在配置好环境变量后进webshell tail -f /var/log/higress/gateway.log。

```jsx
sudo bash -c \"sed -i 's|oci://registry.cn-hangzhou.aliyuncs.com/XXX:[0-9]*\\\\.[0-9]*\\\\.[0-9]*|oci://registry.cn-hangzhou.aliyuncs.com/XXX:$(cat version.txt)|g' data/wasmplugins/ai-cache-1.0.0.yaml\
```

并且建议把log目录也挂载到NAS里防止白跑。（参考本地环境配置的yaml)

这里还有个小trick和大家分享讨论，如何保证每次的环境里的数据库都是最新的。如果每次都删库重建是可以的，就是会有初始化来不及和忘记删的问题。我刚开始是在ParseConfig中初始化唯一的session_uuid作为对应，在请求向量数据库和redis的时候加上该field。（如何用field过滤在dashvector中有提，是写个附带的sql语法）。但是由于ParseConfig函数并不是只执行一次，我换成了在更新配置的时候手动写入。这样本地写个脚本生成需要执行的sed命令即可。

**最后恭祝大家能取得好成绩，有问题可以在群里@我（苏淳sv），或者在github上发issue。**