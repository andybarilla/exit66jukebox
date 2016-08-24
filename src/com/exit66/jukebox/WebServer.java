package com.exit66.jukebox;

import org.eclipse.jetty.server.Server;
import org.eclipse.jetty.servlet.*;

public class WebServer extends Thread {
    
    boolean _keepRunning = true;
    
    private static WebServer ref;
    protected Server _server;
    
	public static WebServer getWebServer()
    throws NullPointerException {
        if (ref == null)
            throw new NullPointerException();
        return ref;
    }
    
    public static WebServer getWebServer(int servPort, String httpRoot) {
        if (ref == null)
            ref = new WebServer(servPort, httpRoot);
        return ref;
    }
    
    public Object clone()
    throws CloneNotSupportedException {
        throw new CloneNotSupportedException();
    }
    
    
    private WebServer(int servPort, String httpRoot) {
    	try {
	    	setName("Exit66 Webserver");
	    	
			_server = new Server(servPort);
			
			ServletContextHandler context = new ServletContextHandler(ServletContextHandler.SESSIONS);

			context.setResourceBase(httpRoot);
			context.setInitParameter("org.eclipse.jetty.servlet.MaxAge", "31536000");
			
			context.addServlet("org.eclipse.jetty.servlet.DefaultServlet", 
							   "/*");
			context.addServlet("com.exit66.jukebox.servlet.LibraryServlet",
					"/rest/library/*");
			context.addServlet("com.exit66.jukebox.servlet.ArtistServlet",
					"/rest/artist/*");
			context.addServlet("com.exit66.jukebox.servlet.AlbumServlet",
					"/rest/album/*");
			context.addServlet("com.exit66.jukebox.servlet.TrackServlet",
					"/rest/track/*");
			context.addServlet("com.exit66.jukebox.servlet.QueueServlet",
					"/rest/queue/*");
			context.addServlet("com.exit66.jukebox.servlet.StatsServlet",
					"/rest/stats/*");
			
			_server.setHandler(context);
			this.start();
    	}
    	catch (Exception e) {
    		System.err.println(e);
    	}
        
    }
    
    public void run() {
        
        try {
        	
        	_server.start();
            
            while (_keepRunning) {
                
                 Thread.sleep(1000);
                 
            }
            
            _server.stop();
            
        } catch (Exception e) { System.err.println(e); }
        System.out.println("Web server closed");
        
    }
    
    public synchronized void shutDown() {
        _keepRunning = false;
    }
}


