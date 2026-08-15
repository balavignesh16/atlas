package com.atlas.order;

import com.atlas.order.client.InventoryClient;
import com.atlas.order.controller.OrderController;
import com.atlas.order.domain.Order;
import com.atlas.order.dto.CreateOrderRequest;
import com.atlas.order.repository.OrderRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.mockito.Mockito;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.mock.mockito.MockBean;
import org.springframework.boot.test.web.client.TestRestTemplate;
import org.springframework.http.*;
import org.springframework.web.server.ResponseStatusException;

import java.util.ArrayList;
import java.util.List;
import java.util.Set;
import java.util.concurrent.*;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.anyInt;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.Mockito.*;

@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT)
public class OrderServiceIntegrationTests {

    @Autowired
    private TestRestTemplate restTemplate;
    
    @Autowired
    private OrderRepository orderRepository;

    @MockBean
    private InventoryClient inventoryClient;

    @BeforeEach
    void setUp() {
        Mockito.reset(inventoryClient);
        // Default happy path mock
        doNothing().when(inventoryClient).reserveInventory(anyString(), anyInt());
    }

    @Test
    void testCreateOrderSuccess() {
        CreateOrderRequest request = new CreateOrderRequest("P100", 2);
        
        HttpHeaders headers = new HttpHeaders();
        headers.set("X-Correlation-ID", "TEST-CORR-123");
        HttpEntity<CreateOrderRequest> entity = new HttpEntity<>(request, headers);

        ResponseEntity<Order> response = restTemplate.postForEntity("/api/orders", entity, Order.class);

        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.CREATED);
        verify(inventoryClient).reserveInventory("P100", 2);
    }

    @Test
    void testInsufficientInventoryReturns409() {
        doThrow(new ResponseStatusException(HttpStatus.CONFLICT, "Insufficient inventory"))
            .when(inventoryClient).reserveInventory("P300", 1);
            
        CreateOrderRequest request = new CreateOrderRequest("P300", 1);
        ResponseEntity<String> response = restTemplate.postForEntity("/api/orders", request, String.class);
        
        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.CONFLICT);
        assertThat(response.getBody()).contains("Insufficient inventory");
    }

    @Test
    void testUnknownProductReturns404() {
        doThrow(new ResponseStatusException(HttpStatus.NOT_FOUND, "Product not found"))
            .when(inventoryClient).reserveInventory("P999", 1);
            
        CreateOrderRequest request = new CreateOrderRequest("P999", 1);
        ResponseEntity<String> response = restTemplate.postForEntity("/api/orders", request, String.class);
        
        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.NOT_FOUND);
    }

    @Test
    void testInventoryUnavailableReturns503() {
        doThrow(new ResponseStatusException(HttpStatus.SERVICE_UNAVAILABLE, "Inventory service unavailable"))
            .when(inventoryClient).reserveInventory("P200", 1);
            
        CreateOrderRequest request = new CreateOrderRequest("P200", 1);
        ResponseEntity<String> response = restTemplate.postForEntity("/api/orders", request, String.class);
        
        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.SERVICE_UNAVAILABLE);
    }

    @Test
    void testInventoryTimeoutReturns504() {
        doThrow(new ResponseStatusException(HttpStatus.GATEWAY_TIMEOUT, "Inventory service timeout"))
            .when(inventoryClient).reserveInventory("P200", 1);
            
        CreateOrderRequest request = new CreateOrderRequest("P200", 1);
        ResponseEntity<String> response = restTemplate.postForEntity("/api/orders", request, String.class);
        
        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.GATEWAY_TIMEOUT);
    }
    
    @Test
    void testInventory500Returns502() {
        doThrow(new ResponseStatusException(HttpStatus.BAD_GATEWAY, "Inventory service error"))
            .when(inventoryClient).reserveInventory("P200", 1);
            
        CreateOrderRequest request = new CreateOrderRequest("P200", 1);
        ResponseEntity<String> response = restTemplate.postForEntity("/api/orders", request, String.class);
        
        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.BAD_GATEWAY);
    }

    @Test
    void testCompensationOnOrderCreationFailure() {
        // We use a mock of OrderRepository to simulate failure after inventory is reserved
        OrderRepository mockRepository = Mockito.mock(OrderRepository.class);
        when(mockRepository.save(any())).thenThrow(new RuntimeException("Simulated order creation failure"));
        
        // Temporarily replace the real repository with the mock for this test
        OrderController testController = new OrderController(mockRepository, inventoryClient);
        
        CreateOrderRequest request = new CreateOrderRequest("P100", 2);
        try {
            testController.createOrder(request);
        } catch (org.springframework.web.server.ResponseStatusException ex) {
            assertThat(ex.getStatusCode()).isEqualTo(HttpStatus.INTERNAL_SERVER_ERROR);
        }
        
        // Verify reserve was called
        verify(inventoryClient).reserveInventory("P100", 2);
        // Verify release was called (Compensation)
        verify(inventoryClient).releaseInventory("P100", 2);
    }
    
    @Test
    void testCompensationFailsGracefully() {
        // We use a mock of OrderRepository to simulate failure after inventory is reserved
        OrderRepository mockRepository = Mockito.mock(OrderRepository.class);
        when(mockRepository.save(any())).thenThrow(new RuntimeException("Simulated order creation failure"));
        
        // If compensation itself throws, it should still return the original error and not crash.
        doThrow(new RuntimeException("Release failed")).when(inventoryClient).releaseInventory("P100", 2);
        
        OrderController testController = new OrderController(mockRepository, inventoryClient);
        
        CreateOrderRequest request = new CreateOrderRequest("P100", 2);
        try {
            testController.createOrder(request);
        } catch (org.springframework.web.server.ResponseStatusException ex) {
            assertThat(ex.getStatusCode()).isEqualTo(HttpStatus.INTERNAL_SERVER_ERROR);
        }

        verify(inventoryClient).releaseInventory("P100", 2); // Was attempted
    }

    @Test
    void testGetExistingOrder() {
        CreateOrderRequest request = new CreateOrderRequest("P200", 5);
        ResponseEntity<Order> createResponse = restTemplate.postForEntity("/api/orders", request, Order.class);
        String orderId = createResponse.getBody().getOrderId();

        ResponseEntity<Order> getResponse = restTemplate.getForEntity("/api/orders/" + orderId, Order.class);
        assertThat(getResponse.getStatusCode()).isEqualTo(HttpStatus.OK);
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
                orderIds.add(response.getBody().getOrderId());
            } catch (ExecutionException e) {
                throw new RuntimeException(e);
            }
        }
        
        assertThat(orderIds).hasSize(100);
        // Verify inventory was reserved 100 times
        verify(inventoryClient, times(100)).reserveInventory(eq("CONCURRENT-PROD"), eq(1));
    }
}
