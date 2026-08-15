package com.atlas.inventory;

import com.atlas.inventory.domain.InventoryItem;
import com.atlas.inventory.dto.ReleaseRequest;
import com.atlas.inventory.dto.ReserveRequest;
import com.atlas.inventory.repository.InventoryRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.web.client.TestRestTemplate;
import org.springframework.http.*;

import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.*;
import java.util.concurrent.atomic.AtomicInteger;

import static org.assertj.core.api.Assertions.assertThat;

@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT)
public class InventoryServiceTests {

    @Autowired
    private TestRestTemplate restTemplate;

    @Autowired
    private InventoryRepository inventoryRepository;

    @BeforeEach
    void setUp() {
        inventoryRepository.reset();
    }

    @Test
    void testGetExistingProduct() {
        ResponseEntity<InventoryItem> response = restTemplate.getForEntity("/api/inventory/P100", InventoryItem.class);
        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.OK);
        assertThat(response.getBody().getProductId()).isEqualTo("P100");
        assertThat(response.getBody().getAvailableQuantity()).isEqualTo(10);
    }

    @Test
    void testGetUnknownProduct() {
        ResponseEntity<String> response = restTemplate.getForEntity("/api/inventory/P999", String.class);
        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.NOT_FOUND);
        assertThat(response.getBody()).contains("PRODUCT_NOT_FOUND");
    }

    @Test
    void testSuccessfulReservation() {
        ReserveRequest request = new ReserveRequest(3);
        ResponseEntity<String> response = restTemplate.postForEntity("/api/inventory/P100/reserve", request, String.class);
        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.OK);
        
        ResponseEntity<InventoryItem> getResponse = restTemplate.getForEntity("/api/inventory/P100", InventoryItem.class);
        assertThat(getResponse.getBody().getAvailableQuantity()).isEqualTo(7);
    }

    @Test
    void testInsufficientInventory() {
        ReserveRequest request = new ReserveRequest(15);
        ResponseEntity<String> response = restTemplate.postForEntity("/api/inventory/P100/reserve", request, String.class);
        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.CONFLICT);
        assertThat(response.getBody()).contains("INSUFFICIENT_INVENTORY");
    }

    @Test
    void testSuccessfulRelease() {
        ReleaseRequest request = new ReleaseRequest(5);
        ResponseEntity<String> response = restTemplate.postForEntity("/api/inventory/P100/release", request, String.class);
        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.OK);

        ResponseEntity<InventoryItem> getResponse = restTemplate.getForEntity("/api/inventory/P100", InventoryItem.class);
        assertThat(getResponse.getBody().getAvailableQuantity()).isEqualTo(15);
    }
    
    @Test
    void testInvalidQuantity() {
        ReserveRequest request = new ReserveRequest(0);
        ResponseEntity<String> response = restTemplate.postForEntity("/api/inventory/P100/reserve", request, String.class);
        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.BAD_REQUEST);
        assertThat(response.getBody()).contains("VALIDATION_ERROR");
    }

    @Test
    void testUnknownProductReservation() {
        ReserveRequest request = new ReserveRequest(1);
        ResponseEntity<String> response = restTemplate.postForEntity("/api/inventory/P999/reserve", request, String.class);
        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.NOT_FOUND);
    }

    @Test
    void testUnknownProductRelease() {
        ReleaseRequest request = new ReleaseRequest(1);
        ResponseEntity<String> response = restTemplate.postForEntity("/api/inventory/P999/release", request, String.class);
        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.NOT_FOUND);
    }

    @Test
    void testConcurrentReservations() throws InterruptedException {
        // P100 starts with 10. Let's reset it to 100 for this test.
        inventoryRepository.release("P100", 90); // 10 + 90 = 100
        
        int numberOfThreads = 100;
        ExecutorService executorService = Executors.newFixedThreadPool(numberOfThreads);
        CountDownLatch latch = new CountDownLatch(numberOfThreads);
        
        List<Future<ResponseEntity<String>>> futures = new ArrayList<>();

        for (int i = 0; i < numberOfThreads; i++) {
            futures.add(executorService.submit(() -> {
                try {
                    ReserveRequest request = new ReserveRequest(1);
                    return restTemplate.postForEntity("/api/inventory/P100/reserve", request, String.class);
                } finally {
                    latch.countDown();
                }
            }));
        }

        latch.await();
        executorService.shutdown();

        int successes = 0;
        for (Future<ResponseEntity<String>> future : futures) {
            try {
                if (future.get().getStatusCode() == HttpStatus.OK) {
                    successes++;
                }
            } catch (ExecutionException e) {
                throw new RuntimeException(e);
            }
        }
        
        assertThat(successes).isEqualTo(100);
        
        ResponseEntity<InventoryItem> getResponse = restTemplate.getForEntity("/api/inventory/P100", InventoryItem.class);
        assertThat(getResponse.getBody().getAvailableQuantity()).isEqualTo(0);
    }

    @Test
    void testOversubscription() throws InterruptedException {
        // P200 starts with 5.
        int numberOfThreads = 10;
        ExecutorService executorService = Executors.newFixedThreadPool(numberOfThreads);
        CountDownLatch latch = new CountDownLatch(numberOfThreads);
        
        List<Future<ResponseEntity<String>>> futures = new ArrayList<>();

        for (int i = 0; i < numberOfThreads; i++) {
            futures.add(executorService.submit(() -> {
                try {
                    ReserveRequest request = new ReserveRequest(1);
                    return restTemplate.postForEntity("/api/inventory/P200/reserve", request, String.class);
                } finally {
                    latch.countDown();
                }
            }));
        }

        latch.await();
        executorService.shutdown();

        AtomicInteger successes = new AtomicInteger(0);
        AtomicInteger conflicts = new AtomicInteger(0);
        
        for (Future<ResponseEntity<String>> future : futures) {
            try {
                ResponseEntity<String> response = future.get();
                if (response.getStatusCode() == HttpStatus.OK) {
                    successes.incrementAndGet();
                } else if (response.getStatusCode() == HttpStatus.CONFLICT) {
                    conflicts.incrementAndGet();
                }
            } catch (ExecutionException e) {
                throw new RuntimeException(e);
            }
        }
        
        assertThat(successes.get()).isEqualTo(5);
        assertThat(conflicts.get()).isEqualTo(5);
        
        ResponseEntity<InventoryItem> getResponse = restTemplate.getForEntity("/api/inventory/P200", InventoryItem.class);
        assertThat(getResponse.getBody().getAvailableQuantity()).isEqualTo(0);
    }

    @Test
    void testCorrelationIdPreservation() {
        HttpHeaders headers = new HttpHeaders();
        headers.set("X-Correlation-ID", "INV-CORR-123");
        HttpEntity<Void> entity = new HttpEntity<>(headers);

        ResponseEntity<InventoryItem> response = restTemplate.exchange("/api/inventory/P100", HttpMethod.GET, entity, InventoryItem.class);
        assertThat(response.getHeaders().getFirst("X-Correlation-ID")).isEqualTo("INV-CORR-123");
    }

    @Test
    void testGeneratedCorrelationId() {
        ResponseEntity<InventoryItem> response = restTemplate.getForEntity("/api/inventory/P100", InventoryItem.class);
        assertThat(response.getHeaders().getFirst("X-Correlation-ID")).isNotNull().isNotEmpty();
    }
}
