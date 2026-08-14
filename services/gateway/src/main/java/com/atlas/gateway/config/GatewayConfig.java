package com.atlas.gateway.config;

import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.http.client.SimpleClientHttpRequestFactory;
import org.springframework.web.client.RestClient;

import java.time.Duration;

@Configuration
public class GatewayConfig {

    @Value("${order.service.timeout-ms:2000}")
    private int timeoutMs;

    @Value("${order.service.url:http://localhost:8084}")
    private String orderServiceUrl;

    @Bean
    public RestClient restClient() {
        SimpleClientHttpRequestFactory factory = new SimpleClientHttpRequestFactory();
        factory.setConnectTimeout(timeoutMs);
        factory.setReadTimeout(timeoutMs);
        
        return RestClient.builder()
                .baseUrl(orderServiceUrl)
                .requestFactory(factory)
                .build();
    }
}
