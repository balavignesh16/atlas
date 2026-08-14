package com.atlas.gateway;

import com.github.tomakehurst.wiremock.WireMockServer;
import com.github.tomakehurst.wiremock.client.WireMock;
import com.github.tomakehurst.wiremock.core.WireMockConfiguration;
import org.junit.jupiter.api.*;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.web.client.TestRestTemplate;
import org.springframework.http.*;

import static com.github.tomakehurst.wiremock.client.WireMock.*;
import static org.assertj.core.api.Assertions.assertThat;

@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT, 
    properties = {
        "order.service.url=http://localhost:8084",
        "order.service.timeout-ms=500"
    })
public class GatewayIntegrationTests {

    private static WireMockServer wireMockServer;

    @Autowired
    private TestRestTemplate restTemplate;

    @BeforeAll
    static void startWireMock() {
        wireMockServer = new WireMockServer(WireMockConfiguration.wireMockConfig().port(8084));
        wireMockServer.start();
        WireMock.configureFor("localhost", 8084);
    }

    @AfterAll
    static void stopWireMock() {
        if (wireMockServer != null) {
            wireMockServer.stop();
        }
    }

    @AfterEach
    void resetWireMock() {
        wireMockServer.resetAll();
    }

    // 1. POST success
    @Test
    void testPostOrderSuccess() {
        stubFor(post(urlEqualTo("/api/orders"))
                .willReturn(aResponse()
                        .withStatus(201)
                        .withHeader("Content-Type", "application/json")
                        .withBody("{\"orderId\":\"MOCK-001\",\"status\":\"CREATED\"}")));

        HttpHeaders headers = new HttpHeaders();
        headers.setContentType(MediaType.APPLICATION_JSON);
        headers.set("X-Correlation-ID", "TEST-123");
        HttpEntity<String> request = new HttpEntity<>("{\"productId\":\"P100\",\"quantity\":2}", headers);

        ResponseEntity<String> response = restTemplate.postForEntity("/api/orders", request, String.class);

        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.CREATED);
        assertThat(response.getBody()).contains("MOCK-001");
        assertThat(response.getHeaders().getFirst("X-Correlation-ID")).isEqualTo("TEST-123");
        
        verify(postRequestedFor(urlEqualTo("/api/orders"))
                .withHeader("X-Correlation-ID", equalTo("TEST-123"))
                .withRequestBody(equalToJson("{\"productId\":\"P100\",\"quantity\":2}")));
    }

    // 2. GET success
    @Test
    void testGetOrderSuccess() {
        stubFor(get(urlEqualTo("/api/orders/MOCK-001"))
                .willReturn(aResponse()
                        .withStatus(200)
                        .withHeader("Content-Type", "application/json")
                        .withBody("{\"orderId\":\"MOCK-001\",\"status\":\"CREATED\"}")));

        HttpHeaders headers = new HttpHeaders();
        headers.set("X-Correlation-ID", "TEST-GET-123");
        HttpEntity<Void> request = new HttpEntity<>(headers);

        ResponseEntity<String> response = restTemplate.exchange("/api/orders/MOCK-001", HttpMethod.GET, request, String.class);

        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.OK);
        assertThat(response.getBody()).contains("MOCK-001");
        assertThat(response.getHeaders().getFirst("X-Correlation-ID")).isEqualTo("TEST-GET-123");
    }

    // 3. Downstream 400
    @Test
    void testDownstream400() {
        stubFor(post(urlEqualTo("/api/orders"))
                .willReturn(aResponse()
                        .withStatus(400)
                        .withBody("{\"error\":\"Bad Request\"}")));

        HttpHeaders headers = new HttpHeaders();
        headers.setContentType(MediaType.APPLICATION_JSON);
        HttpEntity<String> request = new HttpEntity<>("{}", headers);

        ResponseEntity<String> response = restTemplate.postForEntity("/api/orders", request, String.class);

        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.BAD_REQUEST);
        assertThat(response.getBody()).contains("Bad Request");
    }

    // 4. Downstream 404
    @Test
    void testDownstream404() {
        stubFor(get(urlEqualTo("/api/orders/999"))
                .willReturn(aResponse()
                        .withStatus(404)
                        .withBody("Not Found")));

        ResponseEntity<String> response = restTemplate.getForEntity("/api/orders/999", String.class);

        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.NOT_FOUND);
        assertThat(response.getBody()).contains("Not Found");
    }

    // 5. Downstream 500
    @Test
    void testDownstream500() {
        stubFor(post(urlEqualTo("/api/orders"))
                .willReturn(aResponse()
                        .withStatus(500)
                        .withBody("Internal Server Error")));

        HttpHeaders headers = new HttpHeaders();
        headers.setContentType(MediaType.APPLICATION_JSON);
        HttpEntity<String> request = new HttpEntity<>("{}", headers);

        ResponseEntity<String> response = restTemplate.postForEntity("/api/orders", request, String.class);

        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.BAD_GATEWAY);
        assertThat(response.getBody()).contains("BAD_GATEWAY");
    }

    // 6. Connection Refused
    @Test
    void testConnectionRefused() {
        wireMockServer.stop();
        
        try {
            HttpHeaders headers = new HttpHeaders();
            headers.setContentType(MediaType.APPLICATION_JSON);
            HttpEntity<String> request = new HttpEntity<>("{}", headers);

            ResponseEntity<String> response = restTemplate.postForEntity("/api/orders", request, String.class);

            assertThat(response.getStatusCode()).isEqualTo(HttpStatus.SERVICE_UNAVAILABLE);
            assertThat(response.getBody()).contains("ORDER_SERVICE_UNAVAILABLE");
        } finally {
            wireMockServer.start();
            WireMock.configureFor("localhost", 8084);
        }
    }

    // 7. Downstream timeout
    @Test
    void testDownstreamTimeout() {
        stubFor(post(urlEqualTo("/api/orders"))
                .willReturn(aResponse()
                        .withFixedDelay(1500) // timeout is 500
                        .withStatus(200)));

        HttpHeaders headers = new HttpHeaders();
        headers.setContentType(MediaType.APPLICATION_JSON);
        HttpEntity<String> request = new HttpEntity<>("{}", headers);

        ResponseEntity<String> response = restTemplate.postForEntity("/api/orders", request, String.class);

        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.GATEWAY_TIMEOUT);
        assertThat(response.getBody()).contains("ORDER_SERVICE_TIMEOUT");
    }

    // 8. Missing correlation ID generated
    @Test
    void testMissingCorrelationIdGenerated() {
        stubFor(post(urlEqualTo("/api/orders"))
                .willReturn(aResponse().withStatus(200)));

        HttpHeaders headers = new HttpHeaders();
        headers.setContentType(MediaType.APPLICATION_JSON);
        HttpEntity<String> request = new HttpEntity<>("{}", headers);

        ResponseEntity<String> response = restTemplate.postForEntity("/api/orders", request, String.class);

        String generatedId = response.getHeaders().getFirst("X-Correlation-ID");
        assertThat(generatedId).isNotNull().isNotEmpty();
        
        verify(postRequestedFor(urlEqualTo("/api/orders"))
                .withHeader("X-Correlation-ID", equalTo(generatedId)));
    }
    
    // Gateway remains healthy
    @Test
    void testHealthEndpoint() {
        ResponseEntity<String> response = restTemplate.getForEntity("/actuator/health", String.class);
        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.OK);
        assertThat(response.getBody()).contains("\"status\":\"UP\"");
    }
}
