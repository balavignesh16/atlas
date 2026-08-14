package com.atlas.gateway.filter;

import jakarta.servlet.FilterChain;
import org.junit.jupiter.api.Test;
import org.slf4j.MDC;
import org.springframework.mock.web.MockHttpServletRequest;
import org.springframework.mock.web.MockHttpServletResponse;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.*;

public class CorrelationIdFilterTests {

    @Test
    void testMdcCleanup() throws Exception {
        CorrelationIdFilter filter = new CorrelationIdFilter();
        MockHttpServletRequest request = new MockHttpServletRequest();
        MockHttpServletResponse response = new MockHttpServletResponse();
        FilterChain filterChain = mock(FilterChain.class);

        filter.doFilterInternal(request, response, filterChain);

        verify(filterChain, times(1)).doFilter(request, response);
        assertThat(MDC.get(CorrelationIdFilter.CORRELATION_ID_KEY)).isNull();
        assertThat(response.getHeader(CorrelationIdFilter.CORRELATION_ID_HEADER)).isNotNull();
    }
}
