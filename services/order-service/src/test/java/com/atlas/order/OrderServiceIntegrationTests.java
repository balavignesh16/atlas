package com.atlas.order;

import com.atlas.order.domain.Order;
import com.atlas.order.dto.CreateOrderRequest;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.web.client.TestRestTemplate;
import org.springframework.http.*;

import java.util.ArrayList;
import java.util.List;
import java.util.Set;
import java.util.concurrent.*;

import static org.assertj.core.api.Assertions.assertThat;

@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT)
public class OrderServiceIntegrationTests {

    @Autowired
    private TestRestTemplate restTemplate;

    @Test
    void testCreateOrderSuccess() {
        CreateOrderRequest request = new CreateOrderRequest("P100", 2);
        
        HttpHeaders headers = new HttpHeaders();
        headers.set("X-Correlation-ID", "TEST-CORR-123");
        HttpEntity<CreateOrderRequest> entity = new HttpEntity<>(request, headers);

        ResponseEntity<Order> response = restTemplate.postForEntity("/api/orders", entity, Order.class);

        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.CREATED);
        Order order = response.getBody();
        assertThat(order).isNotNull();
        assertThat(order.getOrderId()).isNotBlank();
        assertThat(order.getProductId()).isEqualTo("P100");
        assertThat(order.getQuantity()).isEqualTo(2);
        assertThat(order.getStatus()).isEqualTo("CREATED");
        assertThat(order.getCreatedAt()).isNotNull();
        
        assertThat(response.getHeaders().getFirst("X-Correlation-ID")).isEqualTo("TEST-CORR-123");
        assertThat(response.getHeaders().getLocation()).isNotNull();
    }

    @Test
    void testGetExistingOrder() {
        CreateOrderRequest request = new CreateOrderRequest("P200", 5);
        ResponseEntity<Order> createResponse = restTemplate.postForEntity("/api/orders", request, Order.class);
        String orderId = createResponse.getBody().getOrderId();

        ResponseEntity<Order> getResponse = restTemplate.getForEntity("/api/orders/" + orderId, Order.class);

        assertThat(getResponse.getStatusCode()).isEqualTo(HttpStatus.OK);
        assertThat(getResponse.getBody().getOrderId()).isEqualTo(orderId);
        assertThat(getResponse.getBody().getProductId()).isEqualTo("P200");
    }

    @Test
    void testGetMissingOrder() {
        ResponseEntity<String> response = restTemplate.getForEntity("/api/orders/NONEXISTENT", String.class);
        
        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.NOT_FOUND);
        assertThat(response.getBody()).contains("ORDER_NOT_FOUND");
    }

    @Test
    void testValidationBlankProductId() {
        CreateOrderRequest request = new CreateOrderRequest("", 1);
        ResponseEntity<String> response = restTemplate.postForEntity("/api/orders", request, String.class);
        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.BAD_REQUEST);
        assertThat(response.getBody()).contains("VALIDATION_ERROR");
    }

    @Test
    void testValidationMissingProductId() {
        CreateOrderRequest request = new CreateOrderRequest(null, 1);
        ResponseEntity<String> response = restTemplate.postForEntity("/api/orders", request, String.class);
        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.BAD_REQUEST);
        assertThat(response.getBody()).contains("VALIDATION_ERROR");
    }

    @Test
    void testValidationZeroQuantity() {
        CreateOrderRequest request = new CreateOrderRequest("P100", 0);
        ResponseEntity<String> response = restTemplate.postForEntity("/api/orders", request, String.class);
        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.BAD_REQUEST);
    }

    @Test
    void testValidationNegativeQuantity() {
        CreateOrderRequest request = new CreateOrderRequest("P100", -1);
        ResponseEntity<String> response = restTemplate.postForEntity("/api/orders", request, String.class);
        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.BAD_REQUEST);
    }

    @Test
    void testValidationMissingQuantity() {
        CreateOrderRequest request = new CreateOrderRequest("P100", null);
        ResponseEntity<String> response = restTemplate.postForEntity("/api/orders", request, String.class);
        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.BAD_REQUEST);
    }

    @Test
    void testMalformedJson() {
        HttpHeaders headers = new HttpHeaders();
        headers.setContentType(MediaType.APPLICATION_JSON);
        HttpEntity<String> entity = new HttpEntity<>("{ invalid json }", headers);

        ResponseEntity<String> response = restTemplate.postForEntity("/api/orders", entity, String.class);
        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.BAD_REQUEST);
        assertThat(response.getBody()).contains("VALIDATION_ERROR");
    }

    @Test
    void testMissingCorrelationIdGeneratesNewOne() {
        CreateOrderRequest request = new CreateOrderRequest("P100", 2);
        HttpEntity<CreateOrderRequest> entity = new HttpEntity<>(request);

        ResponseEntity<Order> response = restTemplate.postForEntity("/api/orders", entity, Order.class);
        String correlationId = response.getHeaders().getFirst("X-Correlation-ID");
        
        assertThat(correlationId).isNotNull().isNotEmpty();
    }

    @Test
    void testConcurrentOrderCreation() throws InterruptedException {
        int numberOfThreads = 100;
        ExecutorService executorService = Executors.newFixedThreadPool(numberOfThreads);
        CountDownLatch latch = new CountDownLatch(numberOfThreads);
        
        Set<String> orderIds = ConcurrentHashMap.newKeySet();
        List<Future<ResponseEntity<Order>>> futures = new ArrayList<>();

        for (int i = 0; i < numberOfThreads; i++) {
            futures.add(executorService.submit(() -> {
                try {
                    CreateOrderRequest request = new CreateOrderRequest("CONCURRENT-PROD", 1);
                    return restTemplate.postForEntity("/api/orders", request, Order.class);
                } finally {
                    latch.countDown();
                }
            }));
        }

        latch.await();
        executorService.shutdown();

        for (Future<ResponseEntity<Order>> future : futures) {
            try {
                ResponseEntity<Order> response = future.get();
                assertThat(response.getStatusCode()).isEqualTo(HttpStatus.CREATED);
                String orderId = response.getBody().getOrderId();
                
                assertThat(orderId).isNotNull();
                assertThat(orderIds.add(orderId)).isTrue(); // Fails if ID is duplicate
            } catch (ExecutionException e) {
                throw new RuntimeException(e);
            }
        }
        
        assertThat(orderIds).hasSize(100);
        
        // Verify all 100 orders can be retrieved
        for (String orderId : orderIds) {
            ResponseEntity<Order> getResponse = restTemplate.getForEntity("/api/orders/" + orderId, Order.class);
            assertThat(getResponse.getStatusCode()).isEqualTo(HttpStatus.OK);
            assertThat(getResponse.getBody().getOrderId()).isEqualTo(orderId);
        }
    }
}
