package com.atlas.payment;

import com.atlas.payment.domain.Payment;
import com.atlas.payment.domain.PaymentStatus;
import com.atlas.payment.dto.PaymentRequest;
import com.atlas.payment.repository.PaymentRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.web.client.TestRestTemplate;
import org.springframework.http.*;

import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.*;

import static org.assertj.core.api.Assertions.assertThat;

@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT)
public class PaymentServiceTests {

    @Autowired
    private TestRestTemplate restTemplate;

    @Autowired
    private PaymentRepository paymentRepository;

    @BeforeEach
    void setUp() {
        paymentRepository.clear();
    }

    @Test
    void testSuccessfulPayment() {
        PaymentRequest request = new PaymentRequest("ORD-001", 1000.0);
        HttpHeaders headers = new HttpHeaders();
        headers.set("Idempotency-Key", "KEY-1");
        
        ResponseEntity<Payment> response = restTemplate.postForEntity("/api/payments", new HttpEntity<>(request, headers), Payment.class);
        
        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.CREATED);
        assertThat(response.getBody()).isNotNull();
        assertThat(response.getBody().getStatus()).isEqualTo(PaymentStatus.AUTHORIZED);
        assertThat(response.getBody().getPaymentId()).isNotNull();
        assertThat(response.getBody().getOrderId()).isEqualTo("ORD-001");
    }
    
    @Test
    void testMissingIdempotencyKey() {
        PaymentRequest request = new PaymentRequest("ORD-001", 1000.0);
        ResponseEntity<String> response = restTemplate.postForEntity("/api/payments", request, String.class);
        
        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.BAD_REQUEST);
        assertThat(response.getBody()).contains("IDEMPOTENCY_KEY_REQUIRED");
    }
    
    @Test
    void testIdempotencyConflict() {
        HttpHeaders headers = new HttpHeaders();
        headers.set("Idempotency-Key", "KEY-2");
        
        PaymentRequest request1 = new PaymentRequest("ORD-001", 1000.0);
        restTemplate.postForEntity("/api/payments", new HttpEntity<>(request1, headers), Payment.class);
        
        PaymentRequest request2 = new PaymentRequest("ORD-002", 5000.0);
        ResponseEntity<String> response = restTemplate.postForEntity("/api/payments", new HttpEntity<>(request2, headers), String.class);
        
        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.CONFLICT);
        assertThat(response.getBody()).contains("IDEMPOTENCY_KEY_REUSED");
    }
    
    @Test
    void testIdempotencySameRequest() {
        HttpHeaders headers = new HttpHeaders();
        headers.set("Idempotency-Key", "KEY-3");
        
        PaymentRequest request = new PaymentRequest("ORD-001", 1000.0);
        
        ResponseEntity<Payment> response1 = restTemplate.postForEntity("/api/payments", new HttpEntity<>(request, headers), Payment.class);
        ResponseEntity<Payment> response2 = restTemplate.postForEntity("/api/payments", new HttpEntity<>(request, headers), Payment.class);
        
        assertThat(response1.getStatusCode()).isEqualTo(HttpStatus.CREATED);
        assertThat(response2.getStatusCode()).isEqualTo(HttpStatus.CREATED);
        assertThat(response1.getBody().getPaymentId()).isEqualTo(response2.getBody().getPaymentId());
    }

    @Test
    void testConcurrentIdempotency() throws InterruptedException, ExecutionException {
        int threads = 100;
        ExecutorService executorService = Executors.newFixedThreadPool(threads);
        List<Callable<ResponseEntity<Payment>>> tasks = new ArrayList<>();
        
        HttpHeaders headers = new HttpHeaders();
        headers.set("Idempotency-Key", "KEY-CONCURRENT");
        PaymentRequest request = new PaymentRequest("ORD-100", 250.0);
        HttpEntity<PaymentRequest> entity = new HttpEntity<>(request, headers);

        for (int i = 0; i < threads; i++) {
            tasks.add(() -> restTemplate.postForEntity("/api/payments", entity, Payment.class));
        }

        List<Future<ResponseEntity<Payment>>> futures = executorService.invokeAll(tasks);
        executorService.shutdown();

        String firstPaymentId = null;
        for (Future<ResponseEntity<Payment>> future : futures) {
            ResponseEntity<Payment> res = future.get();
            assertThat(res.getStatusCode()).isEqualTo(HttpStatus.CREATED);
            if (firstPaymentId == null) {
                firstPaymentId = res.getBody().getPaymentId();
            } else {
                assertThat(res.getBody().getPaymentId()).isEqualTo(firstPaymentId);
            }
        }
    }

    @Test
    void testSandboxDecline() {
        PaymentRequest request = new PaymentRequest("ORD-001", 7777.0);
        HttpHeaders headers = new HttpHeaders();
        headers.set("Idempotency-Key", "KEY-DECLINE");
        
        ResponseEntity<String> response = restTemplate.postForEntity("/api/payments", new HttpEntity<>(request, headers), String.class);
        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.PAYMENT_REQUIRED);
        assertThat(response.getBody()).contains("PAYMENT_DECLINED");
    }
    
    @Test
    void testSandboxServerError() {
        PaymentRequest request = new PaymentRequest("ORD-001", 8888.0);
        HttpHeaders headers = new HttpHeaders();
        headers.set("Idempotency-Key", "KEY-ERROR");
        
        ResponseEntity<String> response = restTemplate.postForEntity("/api/payments", new HttpEntity<>(request, headers), String.class);
        assertThat(response.getStatusCode()).isEqualTo(HttpStatus.INTERNAL_SERVER_ERROR);
    }
}
