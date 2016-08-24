package com.exit66.jukebox.data;

/**
 * @author andyb
 *
 * To change this generated comment edit the template variable "typecomment":
 * Window>Preferences>Java>Templates.
 * To enable and disable the creation of type comments go to
 * Window>Preferences>Java>Code Generation.
 */

import java.sql.SQLException;

public class LibraryCollection extends BVDatabaseCollection {
		
	public LibraryCollection() {
		
		_currentRecord = new Library();
		
	}
		
	public void list() {
		
		if (openConnection()) {
			
			_rs = retrieveData("SELECT * FROM library ORDER BY library_path");
			try {
				
				if (_rs.next()) {
					_eof = false;
				}
				else {
					_eof = true;
				}
				
			}
			catch (SQLException e) {
			
				_eof = true;
					
			}			
			
		}
		
	}
	
	protected void loadCurrent() {
		
		try {
			
			((Library)_currentRecord).setLibraryID(_rs.getInt("library_id"));
			((Library)_currentRecord).setLibraryPath(_rs.getString("library_path"));
			((Library)_currentRecord).setScanStatus(_rs.getInt("scan_status"));

		}
		catch (SQLException e) {};
		
	}

}
