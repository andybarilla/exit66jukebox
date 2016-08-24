package com.exit66.jukebox.data;

/**
 * @author andyb
 *
 * To change this generated comment edit the template variable "typecomment":
 * Window>Preferences>Java>Templates.
 * To enable and disable the creation of type comments go to
 * Window>Preferences>Java>Code Generation.
 */

import java.io.File;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;

public class Library extends BVDatabaseObject {
    
    private int			_libraryID = -1;
    private String 		_libraryPath;
    private int			_scanStatus = 0;
    
    public Library() {
        
        
    }
    
    public int getLibraryID() {
        
        return _libraryID;
        
    }
    
    public void setLibraryID(int newValue) {
        
        _libraryID = newValue;
        
    }
    
    public String getLibraryPath() {
        
        if (_libraryPath.substring(_libraryPath.length() - System.getProperty("file.separator").length()).compareTo(System.getProperty("file.separator")) != 0) {
            
            return _libraryPath.concat(System.getProperty("file.separator"));
            
        } else {
            
            return _libraryPath;
            
        }
        
    }
    
    public void setLibraryPath(String newValue) {
        
        _libraryPath = newValue;
        
    }
    
    public void setScanStatus(int newValue) {
        
        _scanStatus = newValue;
        
    }
    
    public int getScanStatus() {
        
        return _scanStatus;
        
    }
    
    public String getScanStatusString() {
        
        if (_scanStatus == 0) {
            
            return "";
            
        } else {
            
            return "Scanning...";
            
        }
        
    }
    
    
    public boolean fetch() {
        
        if (openConnection()) {
        	
            try {
            	PreparedStatement ps = conn.prepareStatement("SELECT * FROM library WHERE library_id = ?");
            	ps.setInt(1, _libraryID);
            	ResultSet rs = retrieveData(ps);
                    
                if (rs != null && rs.next()) {
                    
                    _libraryPath = rs.getString("library_path");
                    _scanStatus = rs.getInt("scan_status");
                    
                } else {
                    
                    _libraryPath = "";
                    _scanStatus = 0;
                    return false;
                    
                }
                
            } catch (SQLException e) {
                
                System.err.println(e);
                
            }
            
            closeConnection();
            
            return true;
            
        } else {
            
            return false;
            
        }
        
    }
    
    public void scan(boolean scanAll) {
        
        System.out.println("Scanning...\n");
        
        if (_libraryPath.length() > 0) {
                        
            new ScanGroup(_libraryID, scanAll);
            
        }
        
    }
    
    public void scan() {
        
        scan(false);
        
    }
    
    public void save() {
        
        ResultSet rs;
        PreparedStatement ps;
        
        if (openConnection()) {
            
            if (_libraryID == -1) {
                
                try {

                	ps = conn.prepareStatement("SELECT library_id FROM library WHERE library_path = ?");
                	ps.setString(1, _libraryPath);
                	rs = retrieveData(ps);

                    if (rs.next()) {
                        
                        _libraryID = rs.getInt("library_id");
                        
                    }
                    
                } catch (SQLException e) {
                    
                    System.err.println(e);
                    
                }
                
            }
            
            if (_libraryID == -1) {
                
            	
                try {
                	conn.setAutoCommit(false);
                	ps = conn.prepareStatement("INSERT INTO library (library_path, scan_status) VALUES (?, ?)");
                	ps.setString(1, _libraryPath);
                	ps.setInt(2, _scanStatus);
                	executeStatement(ps);
                	
                	rs = retrieveData("CALL IDENTITY()");
                	    
                    if (rs.next()) {
                        
                        _libraryID = rs.getInt(1);
                        
                    }
                    
                    conn.commit();
                    conn.setAutoCommit(true);
                    
                } catch (SQLException e) {
                    
                    System.err.println(e);
                    
                }
                
            } else {
                
            	try {
	            	ps = conn.prepareStatement("UPDATE library SET library_path = ?, scan_status = ?" +
	                        " WHERE library_id = ?");
	            	ps.setString(1, _libraryPath);
	            	ps.setInt(2, _scanStatus);
	            	ps.setInt(3, _libraryID);
	            	ps.execute();
	            	
            	} catch (SQLException e) {
            		
            		System.err.println(e);
            		
            	}
            }
            
            closeConnection();
            
        }
        
    }
    
    public void remove() {
        
        if (openConnection()) {
            
        	try {
	        	PreparedStatement ps = conn.prepareStatement("DELETE FROM library WHERE library_id = ?");
	        	ps.setInt(1, _libraryID);
	        	executeStatement(ps);
	        	
        	} catch (SQLException e) {
        		
        		System.err.println(e);
        		
        	}
        	
            Maintenance maint = new Maintenance();
            maint.cleanDatabase();
            
            closeConnection();
        }
        
    }
    
    public int getTrackCount() {
        
        int count = 0;
        
        if (openConnection()) {
            
        	try {

        		PreparedStatement ps = conn.prepareStatement("SELECT COUNT(*) AS total FROM track WHERE library_id = ?");
        		ps.setInt(1, _libraryID);
        		ResultSet rs = retrieveData(ps);
        		
                if (rs.next()) {
                    
                    count = rs.getInt("total");
                    
                }
                
            } catch (Exception e) {
                
                System.out.println(e);
                
            }
            
            closeConnection();
            
        }
        
        return count;
        
    }
    
    public void addFailedFile(String fullFileName, String fileName) {
        
        if (openConnection()) {
            
            try {
                
            	PreparedStatement ps = conn.prepareStatement("INSERT INTO library_failed_files (library_id, full_file_name, " + 
            			"file_name) VALUES (?, ?, ?)");
            	ps.setInt(1, _libraryID);
            	ps.setString(2, fullFileName);
            	ps.setString(3, fileName);
            	executeStatement(ps);
            	
                System.out.println("File failed: " + fullFileName);
                
            } catch (Exception e) {}
            
        }
        
    }
    
    public void cleanMissingFiles() {
        
        if (_libraryPath.length() == 0) {
            
            fetch();
            
        }
        if (_libraryPath.length() == 0) {
            
            return;
            
        }
        
        System.out.print(_libraryPath);
        
        if (openConnection()) {
            
        	try {

        		PreparedStatement ps = conn.prepareStatement("SELECT * FROM track WHERE library_id = ?");
        		ps.setInt(1, _libraryID);
        		ResultSet rs = retrieveData(ps);
                    
                while (rs.next()) {
                    
                    try {
                        
                        File f = new File(getLibraryPath().concat(rs.getString("file_name")));
                        if (f.exists() == false) {
                            
                        	PreparedStatement psDelete = conn.prepareStatement("DELETE FROM track WHERE track_id = ?");
                        	psDelete.setInt(1, rs.getInt("track_id"));
                        	executeStatement(psDelete);
                            System.out.println(getLibraryPath().concat(rs.getString("file_name")) + " removed from database");
                            
                        }
                        
                    } catch (Exception e) {
                        
                        System.err.println(e);
                        
                    }
                }
                
            } catch (Exception e) {
                
                System.out.println(e);
                
            }
            
            closeConnection();
            
        }
        
    }
    
    public void toJson(StringBuffer sb, int childLevel) {
    	sb.append("{ ");
    	appendJsonElement(sb, "library_id", _libraryID);
    	sb.append(", ");
    	appendJsonElement(sb, "path", _libraryPath);
    	sb.append(", ");
    	appendJsonElement(sb, "scan_status", _scanStatus);
    	sb.append("}");
    }
    
    public static void scanAll() {
    	scanAll(false);
    }
    
    public static void scanAll(boolean scanAll) {
    	LibraryCollection list = new LibraryCollection();
    	list.list();
    	while (!list.isEOF()) {
    		((Library)list.getCurrent()).scan(scanAll);
    		list.moveNext();
    	}    	
    }    
}

