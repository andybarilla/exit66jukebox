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

public class ContentTypeCollection extends BVDatabaseCollection {

	ContentType 		_currentRecord;
	
	public ContentTypeCollection() {
		
		_currentRecord = new ContentType();
		
	}
		
	public void list() {
		
		if (openConnection()) {
			
			_rs = retrieveData("SELECT * FROM content_type ORDER BY content_type");
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
			
			_currentRecord.setContentTypeID(_rs.getInt("content_type_id"));
			_currentRecord.setContentType(_rs.getString("content_type"));
			
		}
		catch (SQLException e) {};
		
	}

}
