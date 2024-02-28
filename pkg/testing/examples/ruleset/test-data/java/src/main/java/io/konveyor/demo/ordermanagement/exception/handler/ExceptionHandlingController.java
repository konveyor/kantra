package io.konveyor.demo.ordermanagement.exception.handler;


import org.jboss.logging.Logger;
import io.konveyor.demo.ordermanagement.exception.ResourceNotFoundException;
import org.springframework.http.HttpStatus;
import org.springframework.web.bind.annotation.ControllerAdvice;
import org.springframework.web.bind.annotation.ExceptionHandler;
import org.springframework.web.bind.annotation.ResponseStatus;

@ControllerAdvice
public class ExceptionHandlingController {
	
	private static Logger logger = Logger.getLogger( ExceptionHandlingController.class.getName() );
	
	@ResponseStatus(value = HttpStatus.NOT_FOUND,
					reason = "Resource not found")
	@ExceptionHandler(ResourceNotFoundException.class)
	public void resourceNotFound(ResourceNotFoundException e) {
		logger.warn("Resource not found: " + e.getMessage());
	}

}
