package com.atlas.gateway.exception;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.ExceptionHandler;
import org.springframework.web.bind.annotation.RestControllerAdvice;
import org.springframework.web.client.HttpClientErrorException;
import org.springframework.web.client.HttpServerErrorException;
import org.springframework.web.client.ResourceAccessException;

import java.net.SocketTimeoutException;

@RestControllerAdvice
public class GlobalExceptionHandler {

    private static final Logger logger = LoggerFactory.getLogger(GlobalExceptionHandler.class);

    @ExceptionHandler(HttpClientErrorException.class)
    public ResponseEntity<String> handleHttpClientError(HttpClientErrorException ex) {
        logger.warn("Downstream returned 4xx error: {}", ex.getStatusCode());
        return ResponseEntity.status(ex.getStatusCode()).body(ex.getResponseBodyAsString());
    }

    @ExceptionHandler(HttpServerErrorException.class)
    public ResponseEntity<GatewayErrorResponse> handleHttpServerError(HttpServerErrorException ex) {
        logger.error("Downstream returned 5xx error: {}", ex.getStatusCode(), ex);
        return ResponseEntity.status(HttpStatus.BAD_GATEWAY)
                .body(new GatewayErrorResponse("BAD_GATEWAY", "Unexpected downstream failure"));
    }

    @ExceptionHandler(org.springframework.web.client.RestClientException.class)
    public ResponseEntity<GatewayErrorResponse> handleRestClientException(org.springframework.web.client.RestClientException ex) {
        logger.error("RestClient exception: {}", ex.getMessage(), ex);
        boolean isTimeout = false;
        
        Throwable cause = ex.getCause();
        while (cause != null) {
            if (cause instanceof SocketTimeoutException || (cause.getMessage() != null && cause.getMessage().toLowerCase().contains("timed out"))) {
                isTimeout = true;
                break;
            }
            cause = cause.getCause();
        }

        if (!isTimeout && ex.getMessage() != null && (ex.getMessage().toLowerCase().contains("timeout") || ex.getMessage().toLowerCase().contains("timed out"))) {
            isTimeout = true;
        }

        if (isTimeout) {
            return ResponseEntity.status(HttpStatus.GATEWAY_TIMEOUT)
                    .body(new GatewayErrorResponse("ORDER_SERVICE_TIMEOUT", "Order service did not respond within the configured timeout"));
        } else {
            return ResponseEntity.status(HttpStatus.SERVICE_UNAVAILABLE)
                    .body(new GatewayErrorResponse("ORDER_SERVICE_UNAVAILABLE", "Order service is currently unavailable"));
        }
    }

    @ExceptionHandler(Exception.class)
    public ResponseEntity<GatewayErrorResponse> handleGenericException(Exception ex) {
        logger.error("Unexpected error occurred", ex);
        return ResponseEntity.status(HttpStatus.INTERNAL_SERVER_ERROR)
                .body(new GatewayErrorResponse("INTERNAL_SERVER_ERROR", "An unexpected error occurred"));
    }
}
