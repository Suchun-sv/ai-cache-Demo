package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/alibaba/higress/plugins/wasm-go/pkg/wrapper"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/tidwall/gjson"
	"github.com/tidwall/resp"
)

const (
	CacheKeyContextKey       = "cacheKey"
	CacheContentContextKey   = "cacheContent"
	PartialMessageContextKey = "partialMessage"
	ToolCallsContextKey      = "toolCalls"
	StreamContextKey         = "stream"
	CacheKeyPrefix           = "higressAiCache"
	DefaultCacheKeyPrefix    = "higressAiCache"
	QueryEmbeddingKey        = "queryEmbedding"
)

func main() {
	wrapper.SetCtx(
		"ai-cache",
		wrapper.ParseConfigBy(parseConfig),
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders),
		wrapper.ProcessRequestBodyBy(onHttpRequestBody),
		wrapper.ProcessResponseHeadersBy(onHttpResponseHeaders),
		wrapper.ProcessStreamingResponseBodyBy(onHttpResponseBody),
	)
}

// @Name ai-cache
// @Category protocol
// @Phase AUTHN
// @Priority 10
// @Title zh-CN AI Cache
// @Description zh-CN 大模型结果缓存
// @IconUrl
// @Version 0.1.0
//
//
// @Contact.name suchunsv
// @Contact.url
// @Contact.email

type RedisInfo struct {
	// @Title zh-CN redis 服务名称
	// @Description zh-CN 带服务类型的完整 FQDN 名称，例如 my-redis.dns、redis.my-ns.svc.cluster.local
	ServiceName string `required:"true" yaml:"serviceName" json:"serviceName"`
	// @Title zh-CN redis 服务端口
	// @Description zh-CN 默认值为6379
	ServicePort int `required:"false" yaml:"servicePort" json:"servicePort"`
	// @Title zh-CN 用户名
	// @Description zh-CN 登陆 redis 的用户名，非必填
	Username string `required:"false" yaml:"username" json:"username"`
	// @Title zh-CN 密码
	// @Description zh-CN 登陆 redis 的密码，非必填，可以只填密码
	Password string `required:"false" yaml:"password" json:"password"`
	// @Title zh-CN 请求超时
	// @Description zh-CN 请求 redis 的超时时间，单位为毫秒。默认值是1000，即1秒
	Timeout int `required:"false" yaml:"timeout" json:"timeout"`
}

type DashVectorInfo struct {
	DashScopeServiceName  string             `require:"true" yaml:"DashScopeServiceName" jaon:"DashScopeServiceName"`
	DashScopeKey          string             `require:"true" yaml:"DashScopeKey" jaon:"DashScopeKey"`
	DashVectorServiceName string             `require:"true" yaml:"DashVectorServiceName" jaon:"DashVectorServiceName"`
	DashVectorKey         string             `require:"true" yaml:"DashVectorKey" jaon:"DashVectorKey"`
	DashVectorAuthApiEnd  string             `require:"true" yaml:"DashVectorEnd" jaon:"DashVectorEnd"`
	DashVectorCollection  string             `require:"true" yaml:"DashVectorCollection" jaon:"DashVectorCollection"`
	DashVectorClient      wrapper.HttpClient `yaml:"-" json:"-"`
	DashScopeClient       wrapper.HttpClient `yaml:"-" json:"-"`
}

type KVExtractor struct {
	// @Title zh-CN 从请求 Body 中基于 [GJSON PATH](https://github.com/tidwall/gjson/blob/master/SYNTAX.md) 语法提取字符串
	RequestBody string `required:"false" yaml:"requestBody" json:"requestBody"`
	// @Title zh-CN 从响应 Body 中基于 [GJSON PATH](https://github.com/tidwall/gjson/blob/master/SYNTAX.md) 语法提取字符串
	ResponseBody string `required:"false" yaml:"responseBody" json:"responseBody"`
}

type PluginConfig struct {
	// @Title zh-CN DashVector 阿里云向量搜索引擎
	// @Description zh-CN 调用阿里云的向量搜索引擎
	DashVectorInfo DashVectorInfo `required:"true" yaml:"dashvector" json:"dashvector"`
	// @Title zh-CN Redis 地址信息
	// @Description zh-CN 用于存储缓存结果的 Redis 地址
	RedisInfo RedisInfo `required:"true" yaml:"redis" json:"redis"`
	// @Title zh-CN 缓存 key 的来源
	// @Description zh-CN 往 redis 里存时，使用的 key 的提取方式
	CacheKeyFrom KVExtractor `required:"true" yaml:"cacheKeyFrom" json:"cacheKeyFrom"`
	// @Title zh-CN 缓存 value 的来源
	// @Description zh-CN 往 redis 里存时，使用的 value 的提取方式
	CacheValueFrom KVExtractor `required:"true" yaml:"cacheValueFrom" json:"cacheValueFrom"`
	// @Title zh-CN 流式响应下，缓存 value 的来源
	// @Description zh-CN 往 redis 里存时，使用的 value 的提取方式
	CacheStreamValueFrom KVExtractor `required:"true" yaml:"cacheStreamValueFrom" json:"cacheStreamValueFrom"`
	// @Title zh-CN 返回 HTTP 响应的模版
	// @Description zh-CN 用 %s 标记需要被 cache value 替换的部分
	ReturnResponseTemplate string `required:"true" yaml:"returnResponseTemplate" json:"returnResponseTemplate"`
	// @Title zh-CN 返回流式 HTTP 响应的模版
	// @Description zh-CN 用 %s 标记需要被 cache value 替换的部分
	ReturnStreamResponseTemplate string `required:"true" yaml:"returnStreamResponseTemplate" json:"returnStreamResponseTemplate"`
	// @Title zh-CN 缓存的过期时间
	// @Description zh-CN 单位是秒，默认值为0，即永不过期
	CacheTTL int `required:"false" yaml:"cacheTTL" json:"cacheTTL"`
	// @Title zh-CN Redis缓存Key的前缀
	// @Description zh-CN 默认值是"higress-ai-cache:"
	CacheKeyPrefix string              `required:"false" yaml:"cacheKeyPrefix" json:"cacheKeyPrefix"`
	redisClient    wrapper.RedisClient `yaml:"-" json:"-"`
}

type Embedding struct {
	Embedding []float64 `json:"embedding"`
	TextIndex int       `json:"text_index"`
}

type Input struct {
	Texts []string `json:"texts"`
}

type Params struct {
	TextType string `json:"text_type"`
}

type Response struct {
	RequestID string `json:"request_id"`
	Output    Output `json:"output"`
	Usage     Usage  `json:"usage"`
}

type Output struct {
	Embeddings []Embedding `json:"embeddings"`
}

type Usage struct {
	TotalTokens int `json:"total_tokens"`
}

// EmbeddingRequest 定义请求的数据结构
type EmbeddingRequest struct {
	Model      string `json:"model"`
	Input      Input  `json:"input"`
	Parameters Params `json:"parameters"`
}

// Document 定义每个文档的结构
type Document struct {
	// ID     string            `json:"id"`
	Vector []float64         `json:"vector"`
	Fields map[string]string `json:"fields"`
}

// InsertRequest 定义插入请求的结构
type InsertRequest struct {
	Docs []Document `json:"docs"`
}

func ConstructTextEmbeddingParameters(c *PluginConfig, log wrapper.Log, texts []string) (string, []byte, [][2]string) {
	url := "/api/v1/services/embeddings/text-embedding/text-embedding"

	data := EmbeddingRequest{
		Model: "text-embedding-v1",
		Input: Input{
			Texts: texts,
		},
		Parameters: Params{
			TextType: "query",
		},
	}

	requestBody, err := json.Marshal(data)
	// requestBody := data
	if err != nil {
		log.Errorf("Failed to marshal request data: %v", err)
		return "", nil, nil
	}

	headers := [][2]string{
		{"Authorization", "Bearer " + c.DashVectorInfo.DashScopeKey},
		{"Content-Type", "application/json"},
	}
	return url, requestBody, headers
}

// QueryResponse 定义查询响应的结构
type QueryResponse struct {
	Code      int      `json:"code"`
	RequestID string   `json:"request_id"`
	Message   string   `json:"message"`
	Output    []Result `json:"output"`
}

// QueryRequest 定义查询请求的结构
type QueryRequest struct {
	Vector        []float64 `json:"vector"`
	TopK          int       `json:"topk"`
	IncludeVector bool      `json:"include_vector"`
}

// Result 定义查询结果的结构
type Result struct {
	ID     string                 `json:"id"`
	Vector []float64              `json:"vector,omitempty"` // omitempty 使得如果 vector 是空，它将不会被序列化
	Fields map[string]interface{} `json:"fields"`
	Score  float64                `json:"score"`
}

func ParseTextEmbedding(responseBody []byte) (*Response, error) {
	var resp Response
	err := json.Unmarshal(responseBody, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func ConstructEmbeddingQueryParameters(c PluginConfig, vector []float64) (string, []byte, [][2]string, error) {
	url := fmt.Sprintf("/v1/collections/%s/query", c.DashVectorInfo.DashVectorCollection)

	requestData := QueryRequest{
		Vector:        vector,
		TopK:          1,
		IncludeVector: false,
	}

	requestBody, err := json.Marshal(requestData)
	if err != nil {
		return "", nil, nil, err
	}

	header := [][2]string{
		{"Content-Type", "application/json"},
		{"dashvector-auth-token", c.DashVectorInfo.DashVectorKey},
	}

	return url, requestBody, header, nil
}

func ParseQueryResponse(responseBody []byte) (*QueryResponse, error) {
	var resp QueryResponse
	err := json.Unmarshal(responseBody, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func ConsturctEmbeddingInsertParameters(c *PluginConfig, log wrapper.Log, text_embedding []float64, query string) (string, []byte, error) {
	// url := fmt.Sprintf("%s/%s/docs", c.DashVectorInfo.DashVectorAuthApiEnd, c.DashVectorInfo.DashVectorCollection)
	url := "/v1/collections/" + c.DashVectorInfo.DashVectorCollection + "/docs"

	key_doc := Document{
		Vector: text_embedding,
		Fields: map[string]string{
			"query": query,
		},
	}

	body, err := json.Marshal(InsertRequest{Docs: []Document{key_doc}})
	if err != nil {
		log.Errorf("Failed to marshal request data: %v", err)
	}

	return url, body, err
}

func parseConfig(json gjson.Result, c *PluginConfig, log wrapper.Log) error {
	log.Infof("config:%s", json.Raw)
	c.DashVectorInfo.DashScopeKey = json.Get("dashvector.DashScopeKey").String()
	log.Infof("dash scope key:%s", c.DashVectorInfo.DashScopeKey)
	if c.DashVectorInfo.DashScopeKey == "" {
		return errors.New("dash scope key must not by empty")
	}
	log.Infof("dash scope key:%s", c.DashVectorInfo.DashScopeKey)
	c.DashVectorInfo.DashScopeServiceName = json.Get("dashvector.DashScopeServiceName").String()
	c.DashVectorInfo.DashVectorServiceName = json.Get("dashvector.DashVectorServiceName").String()
	log.Infof("dash vector service name:%s", c.DashVectorInfo.DashVectorServiceName)
	c.DashVectorInfo.DashVectorKey = json.Get("dashvector.DashVectorKey").String()
	log.Infof("dash vector key:%s", c.DashVectorInfo.DashVectorKey)
	if c.DashVectorInfo.DashVectorKey == "" {
		return errors.New("dash vector key must not by empty")
	}
	c.DashVectorInfo.DashVectorAuthApiEnd = json.Get("dashvector.DashVectorEnd").String()
	log.Infof("dash vector end:%s", c.DashVectorInfo.DashVectorAuthApiEnd)
	if c.DashVectorInfo.DashVectorAuthApiEnd == "" {
		return errors.New("dash vector end must not by empty")
	}
	c.DashVectorInfo.DashVectorCollection = json.Get("dashvector.DashVectorCollection").String()
	log.Infof("dash vector collection:%s", c.DashVectorInfo.DashVectorCollection)

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

	c.RedisInfo.ServiceName = json.Get("redis.serviceName").String()
	if c.RedisInfo.ServiceName == "" {
		return errors.New("redis service name must not by empty")
	}
	c.RedisInfo.ServicePort = int(json.Get("redis.servicePort").Int())
	if c.RedisInfo.ServicePort == 0 {
		if strings.HasSuffix(c.RedisInfo.ServiceName, ".static") {
			// use default logic port which is 80 for static service
			c.RedisInfo.ServicePort = 80
		} else {
			c.RedisInfo.ServicePort = 6379
		}
	}
	c.RedisInfo.Username = json.Get("redis.username").String()
	c.RedisInfo.Password = json.Get("redis.password").String()
	c.RedisInfo.Timeout = int(json.Get("redis.timeout").Int())
	if c.RedisInfo.Timeout == 0 {
		c.RedisInfo.Timeout = 1000
	}
	c.CacheKeyFrom.RequestBody = json.Get("cacheKeyFrom.requestBody").String()
	if c.CacheKeyFrom.RequestBody == "" {
		c.CacheKeyFrom.RequestBody = "messages.@reverse.0.content"
	}
	c.CacheValueFrom.ResponseBody = json.Get("cacheValueFrom.responseBody").String()
	if c.CacheValueFrom.ResponseBody == "" {
		c.CacheValueFrom.ResponseBody = "choices.0.message.content"
	}
	c.CacheStreamValueFrom.ResponseBody = json.Get("cacheStreamValueFrom.responseBody").String()
	if c.CacheStreamValueFrom.ResponseBody == "" {
		c.CacheStreamValueFrom.ResponseBody = "choices.0.delta.content"
	}
	c.ReturnResponseTemplate = json.Get("returnResponseTemplate").String()
	if c.ReturnResponseTemplate == "" {
		c.ReturnResponseTemplate = `{"id":"from-cache","choices":[{"index":0,"message":{"role":"assistant","content":"%s"},"finish_reason":"stop"}],"model":"gpt-4o","object":"chat.completion","usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}`
	}
	c.ReturnStreamResponseTemplate = json.Get("returnStreamResponseTemplate").String()
	if c.ReturnStreamResponseTemplate == "" {
		c.ReturnStreamResponseTemplate = `data:{"id":"from-cache","choices":[{"index":0,"delta":{"role":"assistant","content":"%s"},"finish_reason":"stop"}],"model":"gpt-4o","object":"chat.completion","usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}` + "\n\ndata:[DONE]\n\n"
	}
	c.CacheKeyPrefix = json.Get("cacheKeyPrefix").String()
	if c.CacheKeyPrefix == "" {
		c.CacheKeyPrefix = DefaultCacheKeyPrefix
	}
	c.redisClient = wrapper.NewRedisClusterClient(wrapper.FQDNCluster{
		FQDN: c.RedisInfo.ServiceName,
		Port: int64(c.RedisInfo.ServicePort),
	})
	return c.redisClient.Init(c.RedisInfo.Username, c.RedisInfo.Password, int64(c.RedisInfo.Timeout))
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config PluginConfig, log wrapper.Log) types.Action {
	contentType, _ := proxywasm.GetHttpRequestHeader("content-type")
	// The request does not have a body.
	if contentType == "" {
		return types.ActionContinue
	}
	if !strings.Contains(contentType, "application/json") {
		log.Warnf("content is not json, can't process:%s", contentType)
		ctx.DontReadRequestBody()
		return types.ActionContinue
	}
	proxywasm.RemoveHttpRequestHeader("Accept-Encoding")
	// The request has a body and requires delaying the header transmission until a cache miss occurs,
	// at which point the header should be sent.
	return types.HeaderStopIteration
}

func TrimQuote(source string) string {
	return strings.Trim(source, `"`)
}

// ===================== 以下是主要逻辑 =====================
// 主handler函数，根据key从redis中获取value ，如果不命中，则首先调用文本向量化接口向量化query，然后调用向量搜索接口搜索最相似的出现过的key，最后再次调用redis获取结果
// 可以把所有handler单独提取为文件，这里为了方便读者复制就和主逻辑放在一个文件中了
// 
// 1. query 进来和 redis 中存的 key 匹配 (redisSearchHandler) ，若完全一致则直接返回 (handleCacheHit)
// 2. 否则请求 text_embdding 接口将 query 转换为 query_embedding (fetchAndProcessEmbeddings)
// 3. 用 query_embedding 和向量数据库中的向量做 ANN search，返回最接近的 key ，并用阈值过滤 (performQueryAndRespond)
// 4. 若返回结果为空或大于阈值，舍去，本轮 cache 未命中, 最后将 query_embedding 存入向量数据库 (uploadQueryEmbedding)
// 5. 若小于阈值，则再次调用 redis对 most similar key 做匹配。 (redisSearchHandler)
// 7. 在 response 阶段请求 redis 新增key/LLM返回结果

func redisSearchHandler(key string, ctx wrapper.HttpContext, config PluginConfig, log wrapper.Log, stream bool, ifUseEmbedding bool) error {
	err := config.redisClient.Get(config.CacheKeyPrefix+key, func(response resp.Value) {
		if err := response.Error(); err == nil && !response.IsNull() {
			log.Warnf("cache hit, key:%s", key)
			handleCacheHit(key, response, stream, ctx, config, log)
		} else {
			log.Warnf("cache miss, key:%s", key)
			if ifUseEmbedding {
				handleCacheMiss(key, err, response, ctx, config, log, key, stream)
			} else {
				proxywasm.ResumeHttpRequest()
				return
			}
		}
	})
	return err
}

// 简单处理缓存命中的情况, 从redis中获取到value后，直接返回
func handleCacheHit(key string, response resp.Value, stream bool, ctx wrapper.HttpContext, config PluginConfig, log wrapper.Log) {
	log.Warnf("cache hit, key:%s", key)
	ctx.SetContext(CacheKeyContextKey, nil)
	if !stream {
		proxywasm.SendHttpResponse(200, [][2]string{{"content-type", "application/json; charset=utf-8"}}, []byte(fmt.Sprintf(config.ReturnResponseTemplate, response.String())), -1)
	} else {
		proxywasm.SendHttpResponse(200, [][2]string{{"content-type", "text/event-stream; charset=utf-8"}}, []byte(fmt.Sprintf(config.ReturnStreamResponseTemplate, response.String())), -1)
	}
}

// 处理缓存未命中的情况，调用fetchAndProcessEmbeddings函数向量化query
func handleCacheMiss(key string, err error, response resp.Value, ctx wrapper.HttpContext, config PluginConfig, log wrapper.Log, queryString string, stream bool) {
	if err != nil {
		log.Warnf("redis get key:%s failed, err:%v", key, err)
	}
	if response.IsNull() {
		log.Warnf("cache miss, key:%s", key)
	}
	fetchAndProcessEmbeddings(key, ctx, config, log, queryString, stream)
}

// 调用文本向量化接口向量化query, 向量化成功后调用processFetchedEmbeddings函数处理向量化结果
func fetchAndProcessEmbeddings(key string, ctx wrapper.HttpContext, config PluginConfig, log wrapper.Log, queryString string, stream bool) {
	Emb_url, Emb_requestBody, Emb_headers := ConstructTextEmbeddingParameters(&config, log, []string{queryString})
	config.DashVectorInfo.DashScopeClient.Post(
		Emb_url,
		Emb_headers,
		Emb_requestBody,
		func(statusCode int, responseHeaders http.Header, responseBody []byte) {
			// log.Infof("statusCode:%d, responseBody:%s", statusCode, string(responseBody))
			log.Infof("Successfully fetched embeddings for key: %s", key)
			if statusCode != 200 {
				log.Errorf("Failed to fetch embeddings, statusCode: %d, responseBody: %s", statusCode, string(responseBody))
				ctx.SetContext(QueryEmbeddingKey, nil)
				proxywasm.ResumeHttpRequest()
			} else {
				processFetchedEmbeddings(key, responseBody, ctx, config, log, stream)
			}
		},
		10000)
}

// 先将向量化的结果存入上下文ctx变量，其次发起向量搜索请求
func processFetchedEmbeddings(key string, responseBody []byte, ctx wrapper.HttpContext, config PluginConfig, log wrapper.Log, stream bool) {
	text_embedding_raw, _ := ParseTextEmbedding(responseBody)
	text_embedding := text_embedding_raw.Output.Embeddings[0].Embedding
	// ctx.SetContext(CacheKeyContextKey, text_embedding)
	ctx.SetContext(QueryEmbeddingKey, text_embedding)
	ctx.SetContext(CacheKeyContextKey, key)
	performQueryAndRespond(key, text_embedding, ctx, config, log, stream)
}

// 调用向量搜索接口搜索最相似的key，搜索成功后调用redisSearchHandler函数获取最相似的key的结果
func performQueryAndRespond(key string, text_embedding []float64, ctx wrapper.HttpContext, config PluginConfig, log wrapper.Log, stream bool) {
	vector_url, vector_request, vector_headers, err := ConstructEmbeddingQueryParameters(config, text_embedding)
	if err != nil {
		log.Errorf("Failed to perform query, err: %v", err)
		proxywasm.ResumeHttpRequest()
		return
	}
	config.DashVectorInfo.DashVectorClient.Post(
		vector_url,
		vector_headers,
		vector_request,
		func(statusCode int, responseHeaders http.Header, responseBody []byte) {
			log.Infof("statusCode:%d, responseBody:%s", statusCode, string(responseBody))
			query_resp, err_query := ParseQueryResponse(responseBody)
			if err_query != nil {
				log.Errorf("Failed to parse response: %v", err)
				proxywasm.ResumeHttpRequest()
				return
			}
			if len(query_resp.Output) < 1 {
				log.Warnf("query response is empty")
				uploadQueryEmbedding(ctx, config, log, key, text_embedding)
				return
			}
			most_similar_key := query_resp.Output[0].Fields["query"].(string)
			log.Infof("most similar key:%s", most_similar_key)
			most_similar_score := query_resp.Output[0].Score
			if most_similar_score < 0.1 {
				ctx.SetContext(CacheKeyContextKey, nil)
				redisSearchHandler(most_similar_key, ctx, config, log, stream, false)
			} else {
				log.Infof("the most similar key's score is too high, key:%s, score:%f", most_similar_key, most_similar_score)
				uploadQueryEmbedding(ctx, config, log, key, text_embedding)
				proxywasm.ResumeHttpRequest()
				return
			}
		},
		100000)
}

// 未命中cache，则将新的query embedding和对应的key存入向量数据库
func uploadQueryEmbedding(ctx wrapper.HttpContext, config PluginConfig, log wrapper.Log, key string, text_embedding []float64) error {
	vector_url, vector_body, err := ConsturctEmbeddingInsertParameters(&config, log, text_embedding, key)
	if err != nil {
		log.Errorf("Failed to construct embedding insert parameters: %v", err)
		proxywasm.ResumeHttpRequest()
		return nil
	}
	err = config.DashVectorInfo.DashVectorClient.Post(
		vector_url,
		[][2]string{
			{"Content-Type", "application/json"},
			{"dashvector-auth-token", config.DashVectorInfo.DashVectorKey},
		},
		vector_body,
		func(statusCode int, responseHeaders http.Header, responseBody []byte) {
			if statusCode != 200 {
				log.Errorf("Failed to upload query embedding: %s", responseBody)
			} else {
				log.Infof("Successfully uploaded query embedding for key: %s", key)
			}
			proxywasm.ResumeHttpRequest()
		},
		10000,
	)
	if err != nil {
		log.Errorf("Failed to upload query embedding: %v", err)
		proxywasm.ResumeHttpRequest()
		return nil
	}
	return nil
}

// ===================== 以上是主要逻辑 =====================

// 主要修改函数，将原有的redis缓存策略逻辑替换为新的基于向量的缓存策略逻辑。
func onHttpRequestBody(ctx wrapper.HttpContext, config PluginConfig, body []byte, log wrapper.Log) types.Action {
	bodyJson := gjson.ParseBytes(body)
	// TODO: It may be necessary to support stream mode determination for different LLM providers.
	stream := false
	if bodyJson.Get("stream").Bool() {
		stream = true
		ctx.SetContext(StreamContextKey, struct{}{})
	} else if ctx.GetContext(StreamContextKey) != nil {
		stream = true
	}
	// key := TrimQuote(bodyJson.Get(config.CacheKeyFrom.RequestBody).Raw)
	key := bodyJson.Get(config.CacheKeyFrom.RequestBody).String()
	if key == "" {
		log.Debug("parse key from request body failed")
		return types.ActionContinue
	}

	queryString := config.CacheKeyPrefix + key

	err := redisSearchHandler(queryString, ctx, config, log, stream, true)

	if err != nil {
		log.Error("redis access failed")
		return types.ActionContinue
	}
	return types.ActionPause
}

func processSSEMessage(ctx wrapper.HttpContext, config PluginConfig, sseMessage string, log wrapper.Log) string {
	subMessages := strings.Split(sseMessage, "\n")
	var message string
	for _, msg := range subMessages {
		if strings.HasPrefix(msg, "data:") {
			message = msg
			break
		}
	}
	if len(message) < 6 {
		log.Warnf("invalid message:%s", message)
		return ""
	}
	// skip the prefix "data:"
	bodyJson := message[5:]
	if gjson.Get(bodyJson, config.CacheStreamValueFrom.ResponseBody).Exists() {
		tempContentI := ctx.GetContext(CacheContentContextKey)
		if tempContentI == nil {
			content := TrimQuote(gjson.Get(bodyJson, config.CacheStreamValueFrom.ResponseBody).Raw)
			ctx.SetContext(CacheContentContextKey, content)
			return content
		}
		append := TrimQuote(gjson.Get(bodyJson, config.CacheStreamValueFrom.ResponseBody).Raw)
		content := tempContentI.(string) + append
		ctx.SetContext(CacheContentContextKey, content)
		return content
	} else if gjson.Get(bodyJson, "choices.0.delta.content.tool_calls").Exists() {
		// TODO: compatible with other providers
		ctx.SetContext(ToolCallsContextKey, struct{}{})
		return ""
	}
	log.Warnf("unknown message:%s", bodyJson)
	return ""
}

func onHttpResponseHeaders(ctx wrapper.HttpContext, config PluginConfig, log wrapper.Log) types.Action {
	contentType, _ := proxywasm.GetHttpResponseHeader("content-type")
	if strings.Contains(contentType, "text/event-stream") {
		ctx.SetContext(StreamContextKey, struct{}{})
	}
	return types.ActionContinue
}

func onHttpResponseBody(ctx wrapper.HttpContext, config PluginConfig, chunk []byte, isLastChunk bool, log wrapper.Log) []byte {
	// log.Infof("I am here")
	if ctx.GetContext(ToolCallsContextKey) != nil {
		// we should not cache tool call result
		return chunk
	}
	keyI := ctx.GetContext(CacheKeyContextKey)
	// log.Infof("I am here 2: %v", keyI)
	if keyI == nil {
		return chunk
	}
	if !isLastChunk {
		stream := ctx.GetContext(StreamContextKey)
		if stream == nil {
			tempContentI := ctx.GetContext(CacheContentContextKey)
			if tempContentI == nil {
				ctx.SetContext(CacheContentContextKey, chunk)
				return chunk
			}
			tempContent := tempContentI.([]byte)
			tempContent = append(tempContent, chunk...)
			ctx.SetContext(CacheContentContextKey, tempContent)
		} else {
			var partialMessage []byte
			partialMessageI := ctx.GetContext(PartialMessageContextKey)
			if partialMessageI != nil {
				partialMessage = append(partialMessageI.([]byte), chunk...)
			} else {
				partialMessage = chunk
			}
			messages := strings.Split(string(partialMessage), "\n\n")
			for i, msg := range messages {
				if i < len(messages)-1 {
					// process complete message
					processSSEMessage(ctx, config, msg, log)
				}
			}
			if !strings.HasSuffix(string(partialMessage), "\n\n") {
				ctx.SetContext(PartialMessageContextKey, []byte(messages[len(messages)-1]))
			} else {
				ctx.SetContext(PartialMessageContextKey, nil)
			}
		}
		return chunk
	}
	// last chunk
	key := keyI.(string)
	stream := ctx.GetContext(StreamContextKey)
	var value string
	if stream == nil {
		var body []byte
		tempContentI := ctx.GetContext(CacheContentContextKey)
		if tempContentI != nil {
			body = append(tempContentI.([]byte), chunk...)
		} else {
			body = chunk
		}
		bodyJson := gjson.ParseBytes(body)

		value = TrimQuote(bodyJson.Get(config.CacheValueFrom.ResponseBody).Raw)
		if value == "" {
			log.Warnf("parse value from response body failded, body:%s", body)
			return chunk
		}
	} else {
		if len(chunk) > 0 {
			var lastMessage []byte
			partialMessageI := ctx.GetContext(PartialMessageContextKey)
			if partialMessageI != nil {
				lastMessage = append(partialMessageI.([]byte), chunk...)
			} else {
				lastMessage = chunk
			}
			if !strings.HasSuffix(string(lastMessage), "\n\n") {
				log.Warnf("invalid lastMessage:%s", lastMessage)
				return chunk
			}
			// remove the last \n\n
			lastMessage = lastMessage[:len(lastMessage)-2]
			value = processSSEMessage(ctx, config, string(lastMessage), log)
		} else {
			tempContentI := ctx.GetContext(CacheContentContextKey)
			if tempContentI == nil {
				return chunk
			}
			value = tempContentI.(string)
		}
	}
	log.Infof("I am processing cache to redis, key:%s, value:%s", key, value)
	config.redisClient.Set(config.CacheKeyPrefix+key, value, nil)
	if config.CacheTTL != 0 {
		config.redisClient.Expire(config.CacheKeyPrefix+key, config.CacheTTL, nil)
	}
	return chunk
}
