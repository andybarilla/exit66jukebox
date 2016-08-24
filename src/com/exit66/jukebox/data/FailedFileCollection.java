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

public class FailedFileCollection extends BVDatabaseCollection {
	
	FailedFile 		_currentRecord;
	
	public FailedFileCollection() {
		
		_currentRecord = new FailedFile();
		
	}
		
	public void list() {
		
		if (openConnection()) {

			_rs = retrieveData("SELECT * FROM library_failed_files ORDER BY file_name");
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

		closeConnection();

	}
	
	protected void loadCurrent() {
		
		try {
			
			_currentRecord.setLibraryID(_rs.getInt("library_id"));
			_currentRecord.setFileName(_rs.getString("file_name"));

		}
		catch (SQLException e) {};
		
	}

}
