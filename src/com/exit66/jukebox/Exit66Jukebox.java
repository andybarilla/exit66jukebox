package com.exit66.jukebox;

import com.exit66.jukebox.data.BVDatabase;
import com.exit66.jukebox.data.Maintenance;

import java.io.FileOutputStream;
import java.io.InputStreamReader;
import java.io.BufferedReader;
import java.io.IOException;
import java.io.PrintStream;
import java.net.ServerSocket;

/**
 *
 * The main class which runs the JukeBox server.	 Starts up the webserver,
 * sound server.
 *
 * @author	Andrew Barilla
 * @version	2.0
 *
 */
public class Exit66Jukebox {
    
    public static BVDatabase conn;
    
    PrintStream psLog;
    String version = "5.0.0";
    
    /**
     *
     * Class constructor
     *
     * This constructor should only be called by inherited classes since it does
     * not start the server.
     *
     */
    public Exit66Jukebox() {
        
    }
    /**
     *
     * Class constructor
     *
     * @param	noLog	determines if errors and messages should be logged to a text file
     *
     */
    public Exit66Jukebox(boolean noLog) {
        start(noLog);
    }
    
    protected void start(boolean noLog) {
    	Maintenance maint;
        Options.loadOptions();
                
        if (!checkAvailableWebPort()) {
            System.exit(0);
            return;
        }   
        
        if (noLog == false) {
            try {
                psLog = new PrintStream(new FileOutputStream(Options.nextLogFile()));
                System.setOut(psLog);
                System.setErr(psLog);
            } catch (Exception e) {
                System.err.println();
            }
        }
           
        conn = new BVDatabase();
                
        if (!conn.openConnection()) {
            System.exit(0);
            return;
        }
        
        maint = new Maintenance();
        maint.verifyDatabase();
        
        try {
            
            WebServer ws = WebServer.getWebServer(Options.getWebServerPort(), Options.getWebDirectory());
            
            ws.join();
            System.exit(0);
            
        } catch (InterruptedException e) {
            
            System.err.println(e);
            
        } finally {
        }
        
    }
    
    protected String promptForInput(String question) {
        InputStreamReader isr = new InputStreamReader(System.in);
        BufferedReader input = new BufferedReader(isr);
        System.out.println(question);
        
        try {
            return input.readLine();
        }
        catch (IOException e) {
            return "";
        }
    }
    
    public boolean checkAvailableWebPort() {
        
        boolean bContinue = true;
        int newPort = 0;
                
        bContinue = true;
        newPort = 0;
        
        while (bContinue) {
            
            try {
                
                ServerSocket testSrv = new ServerSocket(Options.getWebServerPort());
                testSrv.close();
                bContinue = false;
                
            } catch (IOException e) {
                
                String newValue = promptForInput("The port assigned to the web server (#" + Options.getWebServerPort() +
                        ") is in use.  Please enter another port to use.");
                if (newValue == null) {
                    return false;
                }
                
                while (bContinue) {
                    
                    try {
                        
                        newPort = Integer.parseInt(newValue);
                        
                        if (newPort != 0) {
                            
                            Options.setWebserverPort(newPort);
                            Options.saveOptions();
                            
                        }
                        
                        bContinue = false;
                        
                    } catch (NumberFormatException ne) {
                        
                        newValue = promptForInput("That is not a valid port number.	Please enter a numeric value.");
                        
                        if (newValue == null) {
                            
                            return false;
                            
                        }
                        
                    }
                    
                }
                
                bContinue = true;
                
            }
            
        }
        return true;
    }
    
        
    /**
     *
     * Main entry point.  Accepts nolog as a command line parameter which is passed to
     * the class constructor
     *
     * @param	args	command-line arguments
     *
     * @see	Exit66JukeBox(boolean)
     *
     */
    public static void main(String[] args) {

        boolean noLog = false;
        boolean console = false;

        for (int i=0; i < args.length; i++) {
            if (args[i].equals("nolog")) {
                noLog = true;
            } else if (args[i].equals("console")) {
                console = true;
            }
        }

        if (!console) {
            new Exit66JukeboxGui(noLog);
        } else {
            new Exit66Jukebox(noLog);
        }
        System.exit(0);
    }
}
