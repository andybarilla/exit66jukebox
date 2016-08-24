/*
 * Exit66JukeboxGui.java
 *
 * Created on April 12, 2006, 9:52 AM
 *
 * To change this template, choose Tools | Template Manager
 * and open the template in the editor.
 */

package com.exit66.jukebox;

import java.awt.AWTException;
import java.awt.Image;
import java.awt.MenuItem;
import java.awt.PopupMenu;
import java.awt.SystemTray;
import java.awt.Toolkit;
import java.awt.TrayIcon;
import java.awt.event.ActionEvent;
import java.awt.event.ActionListener;
import java.awt.event.MouseEvent;
import java.awt.event.MouseListener;
import java.lang.reflect.Method;
import javax.swing.*;

/**
 * 
 * @author andyb
 */
public class Exit66JukeboxGui extends Exit66Jukebox {

	/** Creates a new instance of Exit66JukeboxGui */
	public Exit66JukeboxGui(boolean noLog) {
		final String version = this.version;
		final TrayIcon trayIcon;

		try {
			UIManager.setLookAndFeel(UIManager.getSystemLookAndFeelClassName());
		} catch (Exception e) {
			System.err.println(e.getMessage());
		}

		if (SystemTray.isSupported()) {
			SystemTray tray = SystemTray.getSystemTray();
			Image image = Toolkit.getDefaultToolkit().getImage("exit66jbicon.png");

			MouseListener mouseListener = new MouseListener() {

				public void mouseClicked(MouseEvent e) {
					System.out.println("Clicked " + e.getClickCount());
					if (e.getClickCount() == 2) {
						try {
							openURL(getLocalUrl());
						} catch (Exception ex) {
							System.err.println(ex.getMessage());
						}
					}					
				}

				public void mouseEntered(MouseEvent e) {
					
				}

				public void mouseExited(MouseEvent e) {
					
				}

				public void mousePressed(MouseEvent e) {
					
				}

				public void mouseReleased(MouseEvent e) {
					
				}
			};

			ActionListener exitListener = new ActionListener() {
				public void actionPerformed(ActionEvent e) {
					WebServer.getWebServer().shutDown();
				}
			};

			ActionListener aboutListener = new ActionListener() {
				public void actionPerformed(ActionEvent e) {
					String newLine = System.getProperty("line.separator");
					JOptionPane.showMessageDialog(null, "Exit 66 Jukebox v"
							+ version + newLine
							+ "(c) 2001-2011 Andrew Barilla" + newLine
							+ newLine + "http://www.exit66.com/",
							"Exit 66 Jukebox", JOptionPane.INFORMATION_MESSAGE,
							new ImageIcon("exit66jb.jpg"));
				}
			};

			ActionListener startListener = new ActionListener() {
				public void actionPerformed(ActionEvent e) {
					try {
						openURL(getLocalUrl());
					} catch (Exception ex) {
						System.err.println(ex.getMessage());
					}
				}
			};

			PopupMenu popup = new PopupMenu();

			MenuItem exitItem = new MenuItem("Exit");
			exitItem.addActionListener(exitListener);

			MenuItem aboutItem = new MenuItem("About...");
			aboutItem.addActionListener(aboutListener);

			MenuItem startItem = new MenuItem("Launch Exit 66 Jukebox");
			startItem.addActionListener(startListener);

			popup.add(startItem);
			popup.addSeparator();
			popup.add(aboutItem);
			popup.addSeparator();
			popup.add(exitItem);
			
			trayIcon = new TrayIcon(image, "Exit 66 Jukebox", popup);
			trayIcon.setImageAutoSize(true);
			trayIcon.addMouseListener(mouseListener);

			try {
				tray.add(trayIcon);
			} catch (AWTException e) {
				System.err.println("Tray icon could not be added.");
			}
		} else {
			trayIcon = null;
		}

		start(noLog);
	}

	protected String promptForInput(String question) {
		return javax.swing.JOptionPane.showInputDialog(null, question,
				"Exit66 JukeBox", javax.swing.JOptionPane.WARNING_MESSAGE);
	}

	public static void openURL(String url) {

		String osName = System.getProperty("os.name");

		try {
			if (osName.startsWith("Mac OS")) {
				Class macUtils = Class.forName("com.apple.mrj.MRJFileUtils");
				Method openURL = macUtils.getDeclaredMethod("openURL",
						new Class[] { String.class });
				openURL.invoke(null, new Object[] { url });
			} else if (osName.startsWith("Windows"))
				Runtime.getRuntime().exec(
						"rundll32 url.dll,FileProtocolHandler " + url);
			else { // assume Unix or Linux
				String[] browsers = { "firefox", "opera", "konqueror",
						"mozilla", "netscape" };
				String browser = null;
				for (int count = 0; count < browsers.length && browser == null; count++)
					if (Runtime.getRuntime()
							.exec(new String[] { "which", browsers[count] })
							.waitFor() == 0)
						browser = browsers[count];
				if (browser == null)
					throw new Exception("Could not find web browser.");
				else
					Runtime.getRuntime().exec(new String[] { browser, url });
			}
		} catch (Exception e) {
			JOptionPane.showMessageDialog(
					null,
					"Error attempting to launch web browser" + ":\n"
							+ e.getLocalizedMessage());
		}
	}

	private String getLocalUrl() {
		int port = Options.getWebServerPort();

		if (port == 80) {
			return "http://localhost/";
		} else {
			return "http://localhost:" + String.valueOf(port) + "/";
		}
	}

}