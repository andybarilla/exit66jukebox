/*
 * XmlSegment.java
 *
 * Created on August 10, 2005, 3:23 PM
 *
 * To change this template, choose Tools | Options and locate the template under
 * the Source Creation and Management node. Right-click the template and choose
 * Open. You can then make changes to the template in the Source Editor.
 */

package com.exit66.jukebox.util;

/**
 *
 * @author andyb
 */
public class XmlSegment {
    
	protected StringBuffer out;
    
    /** Creates a new instance of XmlSegment */
    public XmlSegment() {
    	out = new StringBuffer();
    }
    
    public String getOutput() {
        return out.toString();
    }
    
    public StringBuffer getOutputBuffer() {
    	return out;
    }
    
    public void startNode(String nodeName) {
        out.append("<" + nodeName + ">");
    }
    
    public void writeNode(String nodeName, String text) {
        out.append("<" + nodeName + ">");
        formatAppendXml(text);
        out.append("</" + nodeName + ">");
    }
    
    public void endNode(String nodeName) {
        out.append("</" + nodeName + ">");
    }
    
    public void appendString(String string) {
    	out.append(string);
    }
            
    public void appendSegment(XmlSegment segment) {
        out.append(segment.getOutputBuffer());
    }
        
    private void formatAppendXml(String val) {
        if (val == null)
        	val = "";
        
        for (int i = 0; i < val.length(); i++) {
            String subst = null;    // value to substitute
            char c = val.charAt( i);
            
            switch ( c) {
                /* five characters with special
                 * meaning to the XML tokeniser */
                case '<':
                    out.append( "&lt;");
                    break;
                case '>':
                	out.append( "&gt;");
                    break;
                case '\'':
                    // clean.append( "&apos;");
                    // 'apos' isn't a valid HTML entity...
                	out.append( "&#39;");
                    break;
                case '\"':
                	out.append( "&quot;");
                    break;
                case '&':
                    /* nasty, nasty... */
                    if ( i + 1 < val.length() && val.charAt( i + 1) == '#')
                        /* looks like a numeric entity - that's OK */
                    	out.append( c);
                    else
                 /* probably a named entity - identify it by
                  * finding it's terminating semi-colon */
                    {
                        int l = val.indexOf( ";", i);
                        if ( l > 0) {
                        	out.append( subst);
                            i = l;
                        } else
                            /* it probably wasn't an entity */
                        	out.append( "&amp;");
                        
                    }
                    
                    break;
                case '\n':
                case '\r':
                case '\t':
                /* assorted control characters we want
                 * to allow */
                	out.append( c);
                    break;
                case '\0':
                	break;
                default:
                    int cn = ( int) c;
                    
                    if ( cn >= 32 && cn < 127)
                    	out.append( c);
                    else
                        if ( cn < 0x10ffff &&
                            !( cn > 0xd800 && cn <= 0xdfff))
                        	out.append( "&#" +
                                    Integer.toString( cn) +
                                    ";");
                /* characters from 32 (space) to 127
                 * (tilde) are old traditional
                 * 7-bit-clean ASCII and will
                 * work. Unicode characters from 128
                 * upwards should all work if
                 * converted to numeric entities apart
                 * from the block 0xd800-0xdfff
                 * (surrogates). Valid Unicode
                 * character codes end at 0x10ffff */
                    
                    break;
            }
        }
    }
 }
