package com.exit66.jukebox.data;

/**
 * @author andyb
 *
 * To change this generated comment edit the template variable "typecomment":
 * Window>Preferences>Java>Templates.
 * To enable and disable the creation of type comments go to
 * Window>Preferences>Java>Code Generation.
 */

import java.util.Properties;

public class FailedFile extends BVDatabaseObject {

	private int			_libraryID = -1;
	private String 		_fileName;
	
	public int getLibraryID() {
		
		return _libraryID;
		
	}
	
	public void setLibraryID(int newValue) {
		
		_libraryID = newValue;
		
	}
	
	public String getFileName() {

		return _fileName;
		
		
	}
	
	public void setFileName(String newValue) {
		
		_fileName = newValue;
		
	}

	public void save() {
		
		if (openConnection()) {
			
			executeStatement("INSERT INTO library_failed_files (library_id, file_name) VALUES (" +
					_libraryID + ", " + Qts(_fileName) + ")");
		
			closeConnection();		
			
		}
		
	}
	
	public void remove() {
		
		if (openConnection()) {
			
			executeStatement("DELETE FROM library_failed_files WHERE library_id = " + _libraryID + 
				" AND file_Name = " + Qts(_fileName));
						
			closeConnection();			
		}
			
	}
	
	public Properties getProperties(Properties prop) {

		prop.setProperty("library_id", String.valueOf(_libraryID));
		prop.setProperty("file_name", noNull(_fileName));

		return prop;
				
	}

    public void toJson(StringBuffer sb, int childLevel) {
    	
    }	
}
